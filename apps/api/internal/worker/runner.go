package worker

import (
	"context"
	"log"
	"time"

	"github.com/fleetdeck/fleetdeck/apps/api/internal/realtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Runner struct {
	pool          *pgxpool.Pool
	hub           *realtime.Hub
	offlineAfter  time.Duration
	rawRetention  time.Duration
	agg5mRetention time.Duration
}

func New(pool *pgxpool.Pool, hub *realtime.Hub, offlineAfter time.Duration, rawDays, agg5mDays int) *Runner {
	return &Runner{
		pool:           pool,
		hub:            hub,
		offlineAfter:   offlineAfter,
		rawRetention:   time.Duration(rawDays) * 24 * time.Hour,
		agg5mRetention: time.Duration(agg5mDays) * 24 * time.Hour,
	}
}

func (r *Runner) Start(ctx context.Context) {
	go r.loop(ctx, 15*time.Second, r.markOffline)
	go r.loop(ctx, 20*time.Second, r.evaluateAlerts)
	go r.loop(ctx, time.Hour, r.retain)
	go r.ensureDefaultRules(ctx)
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

func (r *Runner) evaluateAlerts(ctx context.Context) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, severity, metric, operator, threshold, duration_seconds, cooldown_seconds
		FROM alert_rules WHERE enabled=true`)
	if err != nil {
		return
	}
	defer rows.Close()

	type rule struct {
		ID, Name, Severity, Metric, Operator string
		Threshold                            float64
		Duration, Cooldown                   int
		UUID                                 uuid.UUID
	}
	rules := make([]rule, 0)
	for rows.Next() {
		var rr rule
		if err := rows.Scan(&rr.UUID, &rr.Name, &rr.Severity, &rr.Metric, &rr.Operator, &rr.Threshold, &rr.Duration, &rr.Cooldown); err != nil {
			continue
		}
		rules = append(rules, rr)
	}

	changed := false
	for _, rule := range rules {
		switch rule.Metric {
		case "cpu_pct", "mem_pct", "disk_pct":
			changed = r.evalHostThreshold(ctx, rule.UUID, rule.Name, rule.Severity, rule.Metric, rule.Operator, rule.Threshold, rule.Duration) || changed
		case "server_offline":
			changed = r.evalOffline(ctx, rule.UUID, rule.Name, rule.Severity) || changed
		case "unhealthy_containers":
			changed = r.evalUnhealthyContainers(ctx, rule.UUID, rule.Name, rule.Severity, rule.Threshold) || changed
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

func (r *Runner) evalHostThreshold(ctx context.Context, ruleID uuid.UUID, name, severity, metric, op string, threshold float64, durationSec int) bool {
	// Approximate duration by requiring latest sample to breach; full windowing comes later.
	rows, err := r.pool.Query(ctx, `
		SELECT s.id, s.name,
			m.cpu_pct,
			CASE WHEN m.mem_used_bytes IS NOT NULL AND m.mem_available_bytes IS NOT NULL AND (m.mem_used_bytes+m.mem_available_bytes)>0
				THEN (m.mem_used_bytes::float8 / (m.mem_used_bytes+m.mem_available_bytes)::float8) * 100 ELSE NULL END AS mem_pct,
			CASE WHEN m.disk_total_bytes IS NOT NULL AND m.disk_total_bytes > 0
				THEN (m.disk_used_bytes::float8 / m.disk_total_bytes::float8) * 100 ELSE NULL END AS disk_pct,
			m.ts
		FROM servers s
		JOIN LATERAL (
			SELECT * FROM server_metrics_raw sm WHERE sm.server_id=s.id ORDER BY sm.ts DESC LIMIT 1
		) m ON true
		WHERE s.status='online'`)
	if err != nil {
		return false
	}
	defer rows.Close()
	changed := false
	for rows.Next() {
		var serverID uuid.UUID
		var serverName string
		var cpu, mem, disk *float64
		var ts time.Time
		if err := rows.Scan(&serverID, &serverName, &cpu, &mem, &disk, &ts); err != nil {
			continue
		}
		var value *float64
		switch metric {
		case "cpu_pct":
			value = cpu
		case "mem_pct":
			value = mem
		case "disk_pct":
			value = disk
		}
		if value == nil || !compare(op, *value, threshold) {
			r.resolveAlert(ctx, ruleID, &serverID, name+" recovered on "+serverName)
			continue
		}
		if time.Since(ts) > time.Duration(durationSec)*time.Second*2 {
			// stale metric — don't fire as current
			continue
		}
		msg := name + " on " + serverName
		if r.upsertActiveAlert(ctx, ruleID, severity, &serverID, nil, msg, map[string]any{
			"metric": metric, "value": *value, "threshold": threshold, "server": serverName,
		}) {
			changed = true
		}
	}
	return changed
}

func (r *Runner) evalOffline(ctx context.Context, ruleID uuid.UUID, name, severity string) bool {
	rows, err := r.pool.Query(ctx, `SELECT id, name FROM servers WHERE status='offline'`)
	if err != nil {
		return false
	}
	defer rows.Close()
	changed := false
	seen := map[uuid.UUID]struct{}{}
	for rows.Next() {
		var id uuid.UUID
		var n string
		if err := rows.Scan(&id, &n); err != nil {
			continue
		}
		seen[id] = struct{}{}
		if r.upsertActiveAlert(ctx, ruleID, severity, &id, nil, name+": "+n, map[string]any{"server": n}) {
			changed = true
		}
	}
	// resolve offline alerts for servers no longer offline
	_, _ = r.pool.Exec(ctx, `
		UPDATE alert_instances
		SET status='resolved', resolved_at=now()
		WHERE rule_id=$1 AND status IN ('active','acknowledged')
		  AND server_id IS NOT NULL
		  AND server_id NOT IN (SELECT id FROM servers WHERE status='offline')`, ruleID)
	_ = seen
	return changed
}

func (r *Runner) evalUnhealthyContainers(ctx context.Context, ruleID uuid.UUID, name, severity string, threshold float64) bool {
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
		if count > threshold {
			if r.upsertActiveAlert(ctx, ruleID, severity, &id, nil, name+" on "+n, map[string]any{"count": count, "server": n}) {
				changed = true
			}
		}
	}
	return changed
}

func (r *Runner) upsertActiveAlert(ctx context.Context, ruleID uuid.UUID, severity string, serverID, containerID *uuid.UUID, message string, contextMap map[string]any) bool {
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
	}
}

func (r *Runner) retain(ctx context.Context) {
	rawCut := time.Now().UTC().Add(-r.rawRetention)
	aggCut := time.Now().UTC().Add(-r.agg5mRetention)
	if _, err := r.pool.Exec(ctx, `DELETE FROM server_metrics_raw WHERE ts < $1`, rawCut); err != nil {
		log.Printf("retain server_metrics_raw: %v", err)
	}
	if _, err := r.pool.Exec(ctx, `DELETE FROM container_metrics_raw WHERE ts < $1`, rawCut); err != nil {
		log.Printf("retain container_metrics_raw: %v", err)
	}
	if _, err := r.pool.Exec(ctx, `DELETE FROM server_metrics_5m WHERE bucket < $1`, aggCut); err != nil {
		log.Printf("retain server_metrics_5m: %v", err)
	}
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
}
