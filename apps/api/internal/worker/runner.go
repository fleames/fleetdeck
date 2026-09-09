package worker

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/fleetdeck/fleetdeck/apps/api/internal/realtime"
	"github.com/fleetdeck/fleetdeck/apps/api/internal/secrets"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Runner struct {
	pool           *pgxpool.Pool
	hub            *realtime.Hub
	secrets        *secrets.Store
	offlineAfter   time.Duration
	rawRetention   time.Duration
	agg5mRetention time.Duration
	agg1hRetention time.Duration
	webhookURL     string
}

func New(pool *pgxpool.Pool, hub *realtime.Hub, offlineAfter time.Duration, rawDays, agg5mDays, agg1hDays int, webhookURL string, secretsStore *secrets.Store) *Runner {
	return &Runner{
		pool:           pool,
		hub:            hub,
		secrets:        secretsStore,
		offlineAfter:   offlineAfter,
		rawRetention:   time.Duration(rawDays) * 24 * time.Hour,
		agg5mRetention: time.Duration(agg5mDays) * 24 * time.Hour,
		agg1hRetention: time.Duration(agg1hDays) * 24 * time.Hour,
		webhookURL:     strings.TrimSpace(webhookURL),
	}
}

func (r *Runner) Start(ctx context.Context) {
	go r.loop(ctx, 15*time.Second, func(c context.Context) {
		r.markOffline(c)
		markWorker("offline")
	})
	go r.loop(ctx, 20*time.Second, func(c context.Context) {
		r.evaluateAlerts(c)
		markWorker("alerts")
	})
	go r.loop(ctx, time.Hour, r.retain)
	go r.loop(ctx, 6*time.Hour, func(c context.Context) {
		r.ensureMetricsPartitions(c)
		markWorker("partitions")
	})
	go r.ensureDefaultRules(ctx)
	go r.ensureMetricsPartitions(ctx)
}

func (r *Runner) loop(ctx context.Context, every time.Duration, fn func(context.Context)) {
	t := time.NewTicker(every)
	defer t.Stop()
	fn(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fn(ctx)
		}
	}
}

func (r *Runner) markOffline(ctx context.Context) {
	cutoff := time.Now().UTC().Add(-r.offlineAfter)
	tag, err := r.pool.Exec(ctx, `
		UPDATE servers
		SET status='offline', health_state='offline', updated_at=now()
		WHERE status='online'
		  AND (last_seen_at IS NULL OR last_seen_at < $1)`, cutoff)
	if err != nil {
		log.Printf("worker offline servers: %v", err)
		return
	}
	_, _ = r.pool.Exec(ctx, `
		UPDATE agents
		SET status='offline', updated_at=now()
		WHERE status='online'
		  AND (last_heartbeat_at IS NULL OR last_heartbeat_at < $1)`, cutoff)
	if tag.RowsAffected() > 0 {
		r.hub.Broadcast("servers.updated", map[string]any{"reason": "offline_detection"})
		_, _ = r.pool.Exec(ctx, `
			INSERT INTO infrastructure_events (kind, severity, message, context)
			VALUES ('servers.offline_detected', 'warning', 'One or more servers marked offline due to missed heartbeats.', jsonb_build_object('count', $1::int))`,
			tag.RowsAffected())
	}
}

func (r *Runner) ensureDefaultRules(ctx context.Context) {
	var n int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM alert_rules`).Scan(&n); err != nil || n > 0 {
		return
	}
	defaults := []struct {
		name, severity, metric, operator string
		threshold                        float64
		duration                         int
	}{
		{"CPU high", "warning", "cpu_pct", ">", 90, 300},
		{"RAM high", "warning", "mem_pct", ">", 90, 300},
		{"Disk high", "critical", "disk_pct", ">", 95, 60},
		{"Disk warning", "warning", "disk_pct", ">", 85, 300},
		{"Server offline", "critical", "server_offline", "==", 1, 45},
		{"Container unhealthy", "critical", "unhealthy_containers", ">", 0, 60},
	}
	for _, d := range defaults {
		_, err := r.pool.Exec(ctx, `
			INSERT INTO alert_rules (name, enabled, severity, scope_type, metric, operator, threshold, duration_seconds, cooldown_seconds)
			VALUES ($1, true, $2, 'all', $3, $4, $5, $6, 300)`,
			d.name, d.severity, d.metric, d.operator, d.threshold, d.duration)
		if err != nil {
			log.Printf("seed alert rule %s: %v", d.name, err)
		}
	}
}

type alertRule struct {
	UUID                                 uuid.UUID
	Name, Severity, Metric, Operator     string
	ScopeType                            string
	ScopeIDs                             []uuid.UUID
	Threshold                            float64
	Duration, Cooldown                   int
}

func (r *Runner) evaluateAlerts(ctx context.Context) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, severity, metric, operator, threshold, duration_seconds, cooldown_seconds,
		       COALESCE(scope_type, 'all'), COALESCE(scope_ids, '[]'::jsonb)
		FROM alert_rules WHERE enabled=true`)
	if err != nil {
		return
	}
	defer rows.Close()

	rules := make([]alertRule, 0)
	for rows.Next() {
		var rr alertRule
		var scopeRaw []byte
		if err := rows.Scan(&rr.UUID, &rr.Name, &rr.Severity, &rr.Metric, &rr.Operator, &rr.Threshold, &rr.Duration, &rr.Cooldown, &rr.ScopeType, &scopeRaw); err != nil {
			continue
		}
		rr.ScopeIDs = parseScopeIDs(scopeRaw)
		rules = append(rules, rr)
	}

	changed := false
	for _, rule := range rules {
		switch rule.Metric {
		case "cpu_pct", "mem_pct", "disk_pct":
			changed = r.evalHostThreshold(ctx, rule) || changed
		case "server_offline":
			changed = r.evalOffline(ctx, rule) || changed
		case "unhealthy_containers":
			changed = r.evalUnhealthyContainers(ctx, rule) || changed
		}
	}
	if changed {
		r.hub.Broadcast("alerts.updated", map[string]any{"reason": "evaluation"})
		r.hub.Broadcast("overview.delta", map[string]any{"reason": "alerts"})
	}
}

func CompareOp(op string, value, threshold float64) bool {
	switch op {
	case ">":
		return value > threshold
	case ">=":
		return value >= threshold
	case "<":
		return value < threshold
	case "<=":
		return value <= threshold
	case "==":
		return value == threshold
	default:
		return false
	}
}

func compare(op string, value, threshold float64) bool {
	return CompareOp(op, value, threshold)
}

// SustainedBreach reports whether samples cover the sustain window and all breach.
// durationSec<=0 means fire on a single latest breaching sample.
func SustainedBreach(sampleTimes []time.Time, sampleValues []float64, op string, threshold float64, durationSec int, now time.Time) (breaching bool, latest float64) {
	if len(sampleTimes) == 0 || len(sampleTimes) != len(sampleValues) {
		return false, 0
	}
	latest = sampleValues[len(sampleValues)-1]
	if durationSec <= 0 {
		return compare(op, latest, threshold), latest
	}
	windowStart := now.Add(-time.Duration(durationSec) * time.Second)
	var inWindow []float64
	var oldestInWindow time.Time
	for i, ts := range sampleTimes {
		if ts.Before(windowStart) {
			continue
		}
		if len(inWindow) == 0 {
			oldestInWindow = ts
		}
		inWindow = append(inWindow, sampleValues[i])
	}
	if len(inWindow) == 0 {
		return false, latest
	}
	// Require coverage from near the start of the window (not only a short recent spike).
	if oldestInWindow.After(windowStart.Add(time.Duration(durationSec) * time.Second / 4)) {
		return false, latest
	}
	for _, v := range inWindow {
		if !compare(op, v, threshold) {
			return false, latest
		}
	}
	return true, latest
}

func (r *Runner) evalHostThreshold(ctx context.Context, rule alertRule) bool {
	duration := rule.Duration
	if duration <= 0 {
		duration = 1
	}
	lookback := time.Duration(duration) * time.Second
	if lookback < 2*time.Minute {
		lookback = 2 * time.Minute
	}

	servers, err := r.pool.Query(ctx, `SELECT id, name FROM servers WHERE status='online'`)
	if err != nil {
		return false
	}
	defer servers.Close()

	changed := false
	now := time.Now().UTC()
	for servers.Next() {
		var serverID uuid.UUID
		var serverName string
		if err := servers.Scan(&serverID, &serverName); err != nil {
			continue
		}
		if !ServerInScope(rule.ScopeType, rule.ScopeIDs, serverID) {
			continue
		}

		metricExpr := hostMetricSQL(rule.Metric)
		if metricExpr == "" {
			continue
		}
		q := `
			SELECT ts, ` + metricExpr + ` AS value
			FROM server_metrics_raw
			WHERE server_id=$1 AND ts >= $2
			ORDER BY ts ASC`
		mrows, err := r.pool.Query(ctx, q, serverID, now.Add(-lookback))
		if err != nil {
			continue
		}
		var times []time.Time
		var values []float64
		for mrows.Next() {
			var ts time.Time
			var v *float64
			if err := mrows.Scan(&ts, &v); err != nil || v == nil {
				continue
			}
			times = append(times, ts)
			values = append(values, *v)
		}
		mrows.Close()

		ok, latest := SustainedBreach(times, values, rule.Operator, rule.Threshold, rule.Duration, now)
		if !ok {
			r.resolveAlert(ctx, rule.UUID, &serverID, rule.Name+" recovered on "+serverName)
			continue
		}
		msg := rule.Name + " on " + serverName
		if r.upsertActiveAlert(ctx, rule.UUID, rule.Severity, &serverID, nil, msg, map[string]any{
			"metric": rule.Metric, "value": latest, "threshold": rule.Threshold, "server": serverName,
			"duration_seconds": rule.Duration,
		}, rule.Cooldown) {
			changed = true
		}
	}
	return changed
}

func hostMetricSQL(metric string) string {
	switch metric {
	case "cpu_pct":
		return "cpu_pct"
	case "mem_pct":
		return `CASE WHEN mem_used_bytes IS NOT NULL AND mem_available_bytes IS NOT NULL AND (mem_used_bytes+mem_available_bytes)>0
			THEN (mem_used_bytes::float8 / (mem_used_bytes+mem_available_bytes)::float8) * 100 ELSE NULL END`
	case "disk_pct":
		return `CASE WHEN disk_total_bytes IS NOT NULL AND disk_total_bytes > 0
			THEN (disk_used_bytes::float8 / disk_total_bytes::float8) * 100 ELSE NULL END`
	default:
		return ""
	}
}

func (r *Runner) evalOffline(ctx context.Context, rule alertRule) bool {
	duration := time.Duration(rule.Duration) * time.Second
	if duration <= 0 {
		duration = r.offlineAfter
	}
	cutoff := time.Now().UTC().Add(-duration)
	rows, err := r.pool.Query(ctx, `
		SELECT id, name FROM servers
		WHERE status='offline'
		  AND (last_seen_at IS NULL OR last_seen_at <= $1)`, cutoff)
	if err != nil {
		return false
	}
	defer rows.Close()
	changed := false
	for rows.Next() {
		var id uuid.UUID
		var n string
		if err := rows.Scan(&id, &n); err != nil {
			continue
		}
		if !ServerInScope(rule.ScopeType, rule.ScopeIDs, id) {
			continue
		}
		if r.upsertActiveAlert(ctx, rule.UUID, rule.Severity, &id, nil, rule.Name+": "+n, map[string]any{"server": n}, rule.Cooldown) {
			changed = true
		}
	}
	_, _ = r.pool.Exec(ctx, `
		UPDATE alert_instances
		SET status='resolved', resolved_at=now()
		WHERE rule_id=$1 AND status IN ('active','acknowledged')
		  AND server_id IS NOT NULL
		  AND server_id NOT IN (
		    SELECT id FROM servers
		    WHERE status='offline' AND (last_seen_at IS NULL OR last_seen_at <= $2)
		  )`, rule.UUID, cutoff)
	return changed
}

func (r *Runner) evalUnhealthyContainers(ctx context.Context, rule alertRule) bool {
	rows, err := r.pool.Query(ctx, `
		SELECT s.id, s.name, COUNT(*)::float8
		FROM containers c
		JOIN servers s ON s.id=c.server_id
		WHERE c.health='unhealthy'
		GROUP BY s.id, s.name`)
	if err != nil {
		return false
	}
	defer rows.Close()
	changed := false
	for rows.Next() {
		var id uuid.UUID
		var n string
		var count float64
		if err := rows.Scan(&id, &n, &count); err != nil {
			continue
		}
		if !ServerInScope(rule.ScopeType, rule.ScopeIDs, id) {
			continue
		}
		if compare(rule.Operator, count, rule.Threshold) {
			if r.upsertActiveAlert(ctx, rule.UUID, rule.Severity, &id, nil, rule.Name+" on "+n, map[string]any{"count": count, "server": n}, rule.Cooldown) {
				changed = true
			}
		} else {
			r.resolveAlert(ctx, rule.UUID, &id, rule.Name+" recovered on "+n)
		}
	}
	return changed
}

func CooldownActive(resolvedAt *time.Time, cooldownSec int, now time.Time) bool {
	if cooldownSec <= 0 || resolvedAt == nil {
		return false
	}
	return now.Sub(*resolvedAt) < time.Duration(cooldownSec)*time.Second
}

func (r *Runner) upsertActiveAlert(ctx context.Context, ruleID uuid.UUID, severity string, serverID, containerID *uuid.UUID, message string, contextMap map[string]any, cooldownSec int) bool {
	var existing uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT id FROM alert_instances
		WHERE rule_id=$1 AND status IN ('active','acknowledged')
		  AND server_id IS NOT DISTINCT FROM $2
		  AND container_id IS NOT DISTINCT FROM $3
		LIMIT 1`, ruleID, serverID, containerID).Scan(&existing)
	if err == nil {
		_, _ = r.pool.Exec(ctx, `
			UPDATE alert_instances SET last_seen_at=now(), message=$2, context=COALESCE($3::jsonb, '{}'::jsonb)
			WHERE id=$1`, existing, message, mustJSON(contextMap))
		return false
	}

	if cooldownSec > 0 {
		var resolvedAt *time.Time
		_ = r.pool.QueryRow(ctx, `
			SELECT resolved_at FROM alert_instances
			WHERE rule_id=$1 AND status='resolved'
			  AND server_id IS NOT DISTINCT FROM $2
			  AND container_id IS NOT DISTINCT FROM $3
			  AND resolved_at IS NOT NULL
			ORDER BY resolved_at DESC
			LIMIT 1`, ruleID, serverID, containerID).Scan(&resolvedAt)
		if CooldownActive(resolvedAt, cooldownSec, time.Now().UTC()) {
			return false
		}
	}

	_, err = r.pool.Exec(ctx, `
		INSERT INTO alert_instances (rule_id, severity, status, server_id, container_id, message, context)
		VALUES ($1,$2,'active',$3,$4,$5,COALESCE($6::jsonb,'{}'::jsonb))`,
		ruleID, severity, serverID, containerID, message, mustJSON(contextMap))
	if err != nil {
		return false
	}
	_, _ = r.pool.Exec(ctx, `
		INSERT INTO infrastructure_events (kind, severity, server_id, message, context)
		VALUES ('alert.fired', $1, $2, $3, COALESCE($4::jsonb,'{}'::jsonb))`,
		severity, serverID, message, mustJSON(contextMap))
	r.notifyWebhook(ctx, "alert.fired", severity, serverID, message, contextMap)
	return true
}

func (r *Runner) resolveAlert(ctx context.Context, ruleID uuid.UUID, serverID *uuid.UUID, message string) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE alert_instances
		SET status='resolved', resolved_at=now(), last_seen_at=now()
		WHERE rule_id=$1 AND status IN ('active','acknowledged')
		  AND server_id IS NOT DISTINCT FROM $2`, ruleID, serverID)
	if err == nil && tag.RowsAffected() > 0 {
		_, _ = r.pool.Exec(ctx, `
			INSERT INTO infrastructure_events (kind, severity, server_id, message)
			VALUES ('alert.resolved', 'info', $1, $2)`, serverID, message)
		r.notifyWebhook(ctx, "alert.resolved", "info", serverID, message, map[string]any{"rule_id": ruleID.String()})
	}
}

func (r *Runner) retain(ctx context.Context) {
	rawDays, agg5mDays, agg1hDays := r.retentionDays(ctx)
	rawCut := time.Now().UTC().Add(-time.Duration(rawDays) * 24 * time.Hour)
	agg5mCut := time.Now().UTC().Add(-time.Duration(agg5mDays) * 24 * time.Hour)
	agg1hCut := time.Now().UTC().Add(-time.Duration(agg1hDays) * 24 * time.Hour)

	// Prefer DROP of fully-aged monthly partitions; DELETE covers DEFAULT + partial months + aggs.
	r.ensureMetricsPartitions(ctx)
	r.dropAgedRawPartitions(ctx, rawCut)

	if _, err := r.pool.Exec(ctx, `DELETE FROM server_metrics_raw WHERE ts < $1`, rawCut); err != nil {
		log.Printf("retain server_metrics_raw: %v", err)
	}
	if _, err := r.pool.Exec(ctx, `DELETE FROM container_metrics_raw WHERE ts < $1`, rawCut); err != nil {
		log.Printf("retain container_metrics_raw: %v", err)
	}
	if _, err := r.pool.Exec(ctx, `DELETE FROM server_metrics_5m WHERE bucket < $1`, agg5mCut); err != nil {
		log.Printf("retain server_metrics_5m: %v", err)
	}
	if _, err := r.pool.Exec(ctx, `DELETE FROM server_metrics_1h WHERE bucket < $1`, agg1hCut); err != nil {
		log.Printf("retain server_metrics_1h: %v", err)
	}
	markWorker("retain")
	markWorker("partitions")
	// downsample: insert 5m aggregates for recent raw not yet aggregated
	_, _ = r.pool.Exec(ctx, `
		INSERT INTO server_metrics_5m (bucket, server_id, cpu_pct_avg, cpu_pct_max, mem_used_bytes_avg, mem_used_bytes_max, disk_used_bytes_avg, net_rx_bps_avg, net_tx_bps_avg, samples)
		SELECT date_trunc('minute', ts) - ((EXTRACT(MINUTE FROM ts)::int % 5) * interval '1 minute') AS bucket,
			server_id,
			AVG(cpu_pct), MAX(cpu_pct),
			AVG(mem_used_bytes)::bigint, MAX(mem_used_bytes),
			AVG(disk_used_bytes)::bigint,
			AVG(net_rx_bps)::bigint, AVG(net_tx_bps)::bigint,
			COUNT(*)::int
		FROM server_metrics_raw
		WHERE ts > now() - interval '2 hours'
		GROUP BY 1, 2
		ON CONFLICT (server_id, bucket) DO UPDATE SET
			cpu_pct_avg=EXCLUDED.cpu_pct_avg,
			cpu_pct_max=EXCLUDED.cpu_pct_max,
			mem_used_bytes_avg=EXCLUDED.mem_used_bytes_avg,
			mem_used_bytes_max=EXCLUDED.mem_used_bytes_max,
			disk_used_bytes_avg=EXCLUDED.disk_used_bytes_avg,
			net_rx_bps_avg=EXCLUDED.net_rx_bps_avg,
			net_tx_bps_avg=EXCLUDED.net_tx_bps_avg,
			samples=EXCLUDED.samples`)

	// downsample 5m → 1h for recent buckets
	_, _ = r.pool.Exec(ctx, `
		INSERT INTO server_metrics_1h (bucket, server_id, cpu_pct_avg, cpu_pct_max, mem_used_bytes_avg, mem_used_bytes_max, disk_used_bytes_avg, net_rx_bps_avg, net_tx_bps_avg, samples)
		SELECT date_trunc('hour', bucket) AS bucket,
			server_id,
			AVG(cpu_pct_avg), MAX(cpu_pct_max),
			AVG(mem_used_bytes_avg)::bigint, MAX(mem_used_bytes_max),
			AVG(disk_used_bytes_avg)::bigint,
			AVG(net_rx_bps_avg)::bigint, AVG(net_tx_bps_avg)::bigint,
			SUM(samples)::int
		FROM server_metrics_5m
		WHERE bucket > now() - interval '48 hours'
		GROUP BY 1, 2
		ON CONFLICT (server_id, bucket) DO UPDATE SET
			cpu_pct_avg=EXCLUDED.cpu_pct_avg,
			cpu_pct_max=EXCLUDED.cpu_pct_max,
			mem_used_bytes_avg=EXCLUDED.mem_used_bytes_avg,
			mem_used_bytes_max=EXCLUDED.mem_used_bytes_max,
			disk_used_bytes_avg=EXCLUDED.disk_used_bytes_avg,
			net_rx_bps_avg=EXCLUDED.net_rx_bps_avg,
			net_tx_bps_avg=EXCLUDED.net_tx_bps_avg,
			samples=EXCLUDED.samples`)
}

// retentionDays prefers Settings UI values when present; falls back to env-derived Runner fields.
// Raw retention: DROP fully-aged monthly partitions when present, plus DELETE for DEFAULT/partial months.
func (r *Runner) retentionDays(ctx context.Context) (raw, agg5m, agg1h int) {
	raw = int(r.rawRetention / (24 * time.Hour))
	agg5m = int(r.agg5mRetention / (24 * time.Hour))
	agg1h = int(r.agg1hRetention / (24 * time.Hour))
	raw, agg5m, agg1h = normalizeRetentionDays(raw, agg5m, agg1h)
	var rawJSON []byte
	err := r.pool.QueryRow(ctx, `SELECT value FROM settings WHERE key='metrics'`).Scan(&rawJSON)
	if err != nil || len(rawJSON) == 0 {
		return raw, agg5m, agg1h
	}
	return applyMetricsSettingsJSON(rawJSON, raw, agg5m, agg1h)
}

func normalizeRetentionDays(raw, agg5m, agg1h int) (int, int, int) {
	if raw < 1 {
		raw = 7
	}
	if agg5m < 1 {
		agg5m = 30
	}
	if agg1h < 1 {
		agg1h = 365
	}
	return raw, agg5m, agg1h
}

func applyMetricsSettingsJSON(rawJSON []byte, raw, agg5m, agg1h int) (int, int, int) {
	var m struct {
		RawRetentionDays   *int `json:"raw_retention_days"`
		Agg5mRetentionDays *int `json:"agg_5m_retention_days"`
		Agg1hRetentionDays *int `json:"agg_1h_retention_days"`
	}
	if err := json.Unmarshal(rawJSON, &m); err != nil {
		return raw, agg5m, agg1h
	}
	if m.RawRetentionDays != nil && *m.RawRetentionDays >= 1 {
		raw = *m.RawRetentionDays
	}
	if m.Agg5mRetentionDays != nil && *m.Agg5mRetentionDays >= 1 {
		agg5m = *m.Agg5mRetentionDays
	}
	if m.Agg1hRetentionDays != nil && *m.Agg1hRetentionDays >= 1 {
		agg1h = *m.Agg1hRetentionDays
	}
	return raw, agg5m, agg1h
}
