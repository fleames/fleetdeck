package worker

import (
	"bytes"
	"context"
	"encoding/json"
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
	}
}

var webhookHTTP = &http.Client{Timeout: 5 * time.Second}

func (r *Runner) resolveWebhookURL(ctx context.Context) string {
	url := strings.TrimSpace(r.webhookURL)
	var rawJSON []byte
	if err := r.pool.QueryRow(ctx, `SELECT value FROM settings WHERE key='alerts'`).Scan(&rawJSON); err == nil && len(rawJSON) > 0 {
		var m struct {
			WebhookURL string `json:"webhook_url"`
		}
		if json.Unmarshal(rawJSON, &m) == nil && strings.TrimSpace(m.WebhookURL) != "" {
			url = strings.TrimSpace(m.WebhookURL)
		}
	}
	return url
}

func (r *Runner) notifyWebhook(ctx context.Context, event string, severity string, serverID *uuid.UUID, message string, contextMap map[string]any) {
	url := r.resolveWebhookURL(ctx)
	if url == "" {
		return
	}
	payload := map[string]any{
		"event":     event,
		"severity":  severity,
		"message":   message,
		"ts":        time.Now().UTC().Format(time.RFC3339),
		"context":   contextMap,
		"source":    "fleetdeck",
	}
	if serverID != nil {
		payload["server_id"] = serverID.String()
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		log.Printf("alert webhook: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "FleetDeck-AlertWebhook/1")
	resp, err := webhookHTTP.Do(req)
	if err != nil {
		log.Printf("alert webhook POST: %v", err)
		return
	}
	_ = resp.Body.Close()
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
