package worker

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	maxUptimeProbes     = 20
	probeTickInterval   = 30 * time.Second
	probeMaxConcurrency = 3
)

type uptimeProbe struct {
	ID             uuid.UUID
	Name           string
	Kind           string
	Target         string
	Method         string
	ExpectedStatus int
	IntervalSec    int
	TimeoutMs      int
	FailThreshold  int
	Severity       string
	ConsecutiveFails int
	LastCheckedAt  *time.Time
}

func (r *Runner) runUptimeProbes(ctx context.Context) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, kind, target, method, expected_status, interval_seconds, timeout_ms,
		       fail_threshold, severity, consecutive_fails, last_checked_at
		FROM uptime_probes
		WHERE enabled=true
		ORDER BY name
		LIMIT $1`, maxUptimeProbes)
	if err != nil {
		log.Printf("uptime probes list: %v", err)
		return
	}
	defer rows.Close()

	now := time.Now().UTC()
	due := make([]uptimeProbe, 0)
	for rows.Next() {
		var p uptimeProbe
		if err := rows.Scan(&p.ID, &p.Name, &p.Kind, &p.Target, &p.Method, &p.ExpectedStatus,
			&p.IntervalSec, &p.TimeoutMs, &p.FailThreshold, &p.Severity, &p.ConsecutiveFails, &p.LastCheckedAt); err != nil {
			continue
		}
		if p.IntervalSec < 30 {
			p.IntervalSec = 30
		}
		if p.LastCheckedAt != nil && now.Sub(*p.LastCheckedAt) < time.Duration(p.IntervalSec)*time.Second {
			continue
		}
		due = append(due, p)
	}
	if len(due) == 0 {
		markWorker("probes")
		return
	}

	ruleID, err := r.ensureProbeAlertRule(ctx)
	if err != nil {
		log.Printf("uptime probe rule: %v", err)
		return
	}

	sem := make(chan struct{}, probeMaxConcurrency)
	var wg sync.WaitGroup
	changed := false
	var mu sync.Mutex
	for _, p := range due {
		p := p
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if r.checkOneProbe(ctx, ruleID, p) {
				mu.Lock()
				changed = true
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if changed {
		r.hub.Broadcast("alerts.updated", map[string]any{"reason": "uptime_probes"})
		r.hub.Broadcast("overview.delta", map[string]any{"reason": "uptime_probes"})
	}
	markWorker("probes")
}

func (r *Runner) ensureProbeAlertRule(ctx context.Context) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT id FROM alert_rules WHERE metric='probe_down' ORDER BY created_at ASC LIMIT 1`).Scan(&id)
	if err == nil {
		return id, nil
	}
	err = r.pool.QueryRow(ctx, `
		INSERT INTO alert_rules (name, enabled, severity, scope_type, metric, operator, threshold, duration_seconds, cooldown_seconds)
		VALUES ('Uptime probe down', true, 'warning', 'all', 'probe_down', '==', 1, 0, 300)
		RETURNING id`).Scan(&id)
	return id, err
}

func (r *Runner) checkOneProbe(ctx context.Context, ruleID uuid.UUID, p uptimeProbe) bool {
	timeout := time.Duration(p.TimeoutMs) * time.Millisecond
	if timeout < 500*time.Millisecond {
		timeout = 3 * time.Second
	}
	if timeout > 10*time.Second {
		timeout = 10 * time.Second
	}

	start := time.Now()
	up, errMsg := false, ""
	switch strings.ToLower(p.Kind) {
	case "tcp":
		up, errMsg = probeTCP(ctx, p.Target, timeout)
	default:
		up, errMsg = probeHTTP(ctx, p.Method, p.Target, p.ExpectedStatus, timeout)
	}
	latency := int(time.Since(start).Milliseconds())

	status := "up"
	if !up {
		status = "down"
		if errMsg == "" {
			errMsg = "probe failed"
		}
	}

	var consecutiveFails int
	err := r.pool.QueryRow(ctx, `
		UPDATE uptime_probes SET
			last_checked_at=now(),
			last_latency_ms=$2,
			last_error=$3,
			last_status=$4,
			consecutive_fails=CASE WHEN $5 THEN 0 ELSE consecutive_fails + 1 END,
			consecutive_oks=CASE WHEN $5 THEN consecutive_oks + 1 ELSE 0 END,
			last_changed_at=CASE
				WHEN last_status IS DISTINCT FROM $4 THEN now()
				ELSE last_changed_at
			END,
			updated_at=now()
		WHERE id=$1
		RETURNING consecutive_fails`,
		p.ID, latency, truncateErr(errMsg), status, up,
	).Scan(&consecutiveFails)
	if err != nil {
		log.Printf("uptime probe update %s: %v", p.Name, err)
		return false
	}

	probeID := p.ID
	changed := false
	if !up && consecutiveFails >= p.FailThreshold {
		msg := fmt.Sprintf("Uptime probe %q is down (%s %s)", p.Name, p.Kind, p.Target)
		if r.upsertActiveAlert(ctx, ruleID, p.Severity, nil, nil, &probeID, msg, map[string]any{
			"probe_id": p.ID.String(),
			"name":     p.Name,
			"kind":     p.Kind,
			"target":   p.Target,
			"error":    truncateErr(errMsg),
			"fails":    consecutiveFails,
		}, 300) {
			changed = true
		}
	}
	if up {
		if r.resolveProbeAlert(ctx, ruleID, &probeID, fmt.Sprintf("Uptime probe %q recovered", p.Name)) {
			changed = true
		}
	}
	return changed
}

func (r *Runner) resolveProbeAlert(ctx context.Context, ruleID uuid.UUID, probeID *uuid.UUID, message string) bool {
	tag, err := r.pool.Exec(ctx, `
		UPDATE alert_instances
		SET status='resolved', resolved_at=now(), last_seen_at=now()
		WHERE rule_id=$1 AND status IN ('active','acknowledged')
		  AND probe_id IS NOT DISTINCT FROM $2`, ruleID, probeID)
	if err != nil || tag.RowsAffected() == 0 {
		return false
	}
	_, _ = r.pool.Exec(ctx, `
		INSERT INTO infrastructure_events (kind, severity, message, context)
		VALUES ('alert.resolved', 'info', $1, jsonb_build_object('probe_id', $2::text))`,
		message, probeID.String())
	r.notifyWebhook(ctx, "alert.resolved", "info", nil, message, map[string]any{
		"rule_id":  ruleID.String(),
		"probe_id": probeID.String(),
	})
	return true
}

func probeHTTP(ctx context.Context, method, target string, expected int, timeout time.Duration) (bool, string) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodGet
	}
	u, err := url.Parse(strings.TrimSpace(target))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false, "invalid http target (need absolute URL)"
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false, "http probe supports http/https only"
	}
	if expected <= 0 {
		expected = 200
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return false, err.Error()
	}
	req.Header.Set("User-Agent", "FleetDeck-UptimeProbe/1.0")
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:           http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
			// Keep dials cheap; no connection reuse across sparse probes.
			DisableKeepAlives: true,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
	res, err := client.Do(req)
	if err != nil {
		return false, err.Error()
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64*1024))
	if res.StatusCode != expected {
		return false, fmt.Sprintf("status %d want %d", res.StatusCode, expected)
	}
	return true, ""
}

func probeTCP(ctx context.Context, target string, timeout time.Duration) (bool, string) {
	target = strings.TrimSpace(target)
	if target == "" || !strings.Contains(target, ":") {
		return false, "invalid tcp target (need host:port)"
	}
	// Reject URLs accidentally pasted into tcp probes.
	if strings.Contains(target, "://") {
		return false, "tcp target must be host:port"
	}
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", target)
	if err != nil {
		return false, err.Error()
	}
	_ = conn.Close()
	return true, ""
}

func truncateErr(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		return s[:500] + "…"
	}
	return s
}
