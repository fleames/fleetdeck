package worker

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Status holds last-run timestamps for internal health/self-metrics (same process).
type Status struct {
	mu            sync.RWMutex
	LastRetainAt  time.Time `json:"last_retain_at,omitempty"`
	LastAlertsAt  time.Time `json:"last_alerts_at,omitempty"`
	LastOfflineAt time.Time `json:"last_offline_at,omitempty"`
	LastPartAt    time.Time `json:"last_partitions_at,omitempty"`
	LastProbesAt  time.Time `json:"last_probes_at,omitempty"`
}

var globalStatus Status

// Snapshot returns a copy of worker last-run times.
func Snapshot() Status {
	globalStatus.mu.RLock()
	defer globalStatus.mu.RUnlock()
	return Status{
		LastRetainAt:  globalStatus.LastRetainAt,
		LastAlertsAt:  globalStatus.LastAlertsAt,
		LastOfflineAt: globalStatus.LastOfflineAt,
		LastPartAt:    globalStatus.LastPartAt,
		LastProbesAt:  globalStatus.LastProbesAt,
	}
}

func markWorker(kind string) {
	now := time.Now().UTC()
	globalStatus.mu.Lock()
	defer globalStatus.mu.Unlock()
	switch kind {
	case "retain":
		globalStatus.LastRetainAt = now
	case "alerts":
		globalStatus.LastAlertsAt = now
	case "offline":
		globalStatus.LastOfflineAt = now
	case "partitions":
		globalStatus.LastPartAt = now
	case "probes":
		globalStatus.LastProbesAt = now
	}
}

var webhookHTTP = &http.Client{Timeout: 5 * time.Second}

const (
	secretKindWebhook   = "webhook"
	secretNameAlertHMAC = "alert_signing"
)

type alertWebhookCfg struct {
	WebhookURL    string `json:"webhook_url"`
	WebhookFormat string `json:"webhook_format"` // auto | json | discord | slack
}

func (r *Runner) loadAlertWebhookCfg(ctx context.Context) alertWebhookCfg {
	cfg := alertWebhookCfg{}
	url := strings.TrimSpace(r.webhookURL)
	var rawJSON []byte
	if err := r.pool.QueryRow(ctx, `SELECT value FROM settings WHERE key='alerts'`).Scan(&rawJSON); err == nil && len(rawJSON) > 0 {
		_ = json.Unmarshal(rawJSON, &cfg)
		if strings.TrimSpace(cfg.WebhookURL) != "" {
			url = strings.TrimSpace(cfg.WebhookURL)
		}
	}
	cfg.WebhookURL = url
	cfg.WebhookFormat = strings.ToLower(strings.TrimSpace(cfg.WebhookFormat))
	return cfg
}

func (r *Runner) resolveWebhookURL(ctx context.Context) string {
	return r.loadAlertWebhookCfg(ctx).WebhookURL
}

// DetectWebhookFormat chooses payload shape: explicit setting, else URL heuristics, else generic JSON.
func DetectWebhookFormat(url, explicit string) string {
	switch strings.ToLower(strings.TrimSpace(explicit)) {
	case "json", "discord", "slack":
		return strings.ToLower(strings.TrimSpace(explicit))
	}
	u := strings.ToLower(url)
	if strings.Contains(u, "discord.com/api/webhooks") || strings.Contains(u, "discordapp.com/api/webhooks") {
		return "discord"
	}
	if strings.Contains(u, "hooks.slack.com") {
		return "slack"
	}
	return "json"
}

func discordColor(severity string) int {
	switch strings.ToLower(severity) {
	case "critical", "crit":
		return 0xE74C3C
	case "warning", "warn":
		return 0xF39C12
	case "info":
		return 0x3498DB
	default:
		return 0x95A5A6
	}
}

// BuildWebhookPayload returns the POST body for the given channel format.
func BuildWebhookPayload(format, event, severity, message string, serverID *uuid.UUID, contextMap map[string]any, ts time.Time) ([]byte, error) {
	if contextMap == nil {
		contextMap = map[string]any{}
	}
	tsRFC := ts.UTC().Format(time.RFC3339)
	switch DetectWebhookFormat("", format) {
	case "discord":
		fields := []map[string]any{
			{"name": "Event", "value": event, "inline": true},
			{"name": "Severity", "value": severity, "inline": true},
		}
		if serverID != nil {
			fields = append(fields, map[string]any{"name": "Server", "value": serverID.String(), "inline": false})
		}
		if ruleID, ok := contextMap["rule_id"]; ok {
			fields = append(fields, map[string]any{"name": "Rule", "value": fmt.Sprint(ruleID), "inline": false})
		}
		payload := map[string]any{
			"username": "FleetDeck",
			"embeds": []map[string]any{{
				"title":       message,
				"description": fmt.Sprintf("`%s` · %s", event, severity),
				"color":       discordColor(severity),
				"timestamp":   tsRFC,
				"fields":      fields,
			}},
		}
		return json.Marshal(payload)
	case "slack":
		text := fmt.Sprintf("[FleetDeck] %s (%s): %s", event, severity, message)
		if serverID != nil {
			text += fmt.Sprintf("\nserver_id=%s", serverID.String())
		}
		payload := map[string]any{
			"text": text,
			"blocks": []map[string]any{
				{
					"type": "section",
					"text": map[string]string{"type": "mrkdwn", "text": fmt.Sprintf("*%s* (`%s`)\n%s", event, severity, message)},
				},
			},
		}
		return json.Marshal(payload)
	default:
		payload := map[string]any{
			"event":    event,
			"severity": severity,
			"message":  message,
			"ts":       tsRFC,
			"context":  contextMap,
			"source":   "fleetdeck",
		}
		if serverID != nil {
			payload["server_id"] = serverID.String()
		}
		return json.Marshal(payload)
	}
}

func (r *Runner) alertSigningKey(ctx context.Context) []byte {
	if r.secrets == nil {
		return nil
	}
	plain, err := r.secrets.Get(ctx, secretKindWebhook, secretNameAlertHMAC)
	if err != nil || len(plain) == 0 {
		return nil
	}
	return plain
}

func (r *Runner) notifyWebhook(ctx context.Context, event string, severity string, serverID *uuid.UUID, message string, contextMap map[string]any) {
	cfg := r.loadAlertWebhookCfg(ctx)
	url := cfg.WebhookURL
	if url == "" {
		return
	}
	format := DetectWebhookFormat(url, cfg.WebhookFormat)
	body, err := BuildWebhookPayload(format, event, severity, message, serverID, contextMap, time.Now().UTC())
	if err != nil {
		log.Printf("alert webhook marshal: %v", err)
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		log.Printf("alert webhook: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "FleetDeck-AlertWebhook/1")
	if key := r.alertSigningKey(ctx); len(key) > 0 && format == "json" {
		mac := hmac.New(sha256.New, key)
		_, _ = mac.Write(body)
		req.Header.Set("X-FleetDeck-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := webhookHTTP.Do(req)
	if err != nil {
		log.Printf("alert webhook POST: %v", err)
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 300 {
		log.Printf("alert webhook status %d", resp.StatusCode)
	}
}

// ServerInScope reports whether serverID matches rule scope (all / servers list).
func ServerInScope(scopeType string, scopeIDs []uuid.UUID, serverID uuid.UUID) bool {
	st := strings.ToLower(strings.TrimSpace(scopeType))
	if st == "" || st == "all" || st == "*" {
		return true
	}
	if st != "servers" && st != "server" {
		// Unknown scope types: fail closed (do not evaluate) to avoid accidental fleet-wide fires.
		return false
	}
	if len(scopeIDs) == 0 {
		return false
	}
	for _, id := range scopeIDs {
		if id == serverID {
			return true
		}
	}
	return false
}

func parseScopeIDs(raw []byte) []uuid.UUID {
	if len(raw) == 0 {
		return nil
	}
	var asStrings []string
	if err := json.Unmarshal(raw, &asStrings); err == nil {
		out := make([]uuid.UUID, 0, len(asStrings))
		for _, s := range asStrings {
			id, err := uuid.Parse(strings.TrimSpace(s))
			if err == nil {
				out = append(out, id)
			}
		}
		return out
	}
	var asUUIDs []uuid.UUID
	if err := json.Unmarshal(raw, &asUUIDs); err == nil {
		return asUUIDs
	}
	return nil
}
