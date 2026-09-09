package api

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/fleetdeck/fleetdeck/apps/api/internal/auth"
	"github.com/fleetdeck/fleetdeck/apps/api/internal/httpx"
	"github.com/fleetdeck/fleetdeck/apps/api/internal/worker"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	kind := chi.URLParam(r, "kind")
	format := strings.ToLower(r.URL.Query().Get("format"))
	if format == "" {
		format = "json"
	}
	if format != "json" && format != "csv" {
		httpx.Error(w, http.StatusBadRequest, "validation", "format must be json or csv")
		return
	}

	switch kind {
	case "servers":
		s.exportServers(w, r, format)
	case "containers":
		s.exportContainers(w, r, format)
	case "alerts":
		s.exportAlerts(w, r, format)
	case "events":
		s.exportEvents(w, r, format)
	default:
		httpx.Error(w, http.StatusNotFound, "not_found", "Unknown export kind.")
	}
}

func (s *Server) exportServers(w http.ResponseWriter, r *http.Request, format string) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT id::text, name, hostname, primary_address, os_name, os_version, arch, status, health_state,
		       docker_available::text, COALESCE(last_seen_at::text,''), COALESCE(last_metrics_at::text,'')
		FROM servers ORDER BY name`)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Export failed.")
		return
	}
	defer rows.Close()
	headers := []string{"id", "name", "hostname", "primary_address", "os_name", "os_version", "arch", "status", "health_state", "docker_available", "last_seen_at", "last_metrics_at"}
	writeExport(w, format, "servers", headers, rowsScanAll(rows, len(headers)))
}

func (s *Server) exportContainers(w http.ResponseWriter, r *http.Request, format string) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT c.id::text, s.name, c.name, c.image_ref, c.state, c.health, c.restart_count::text, c.compose_project
		FROM containers c JOIN servers s ON s.id=c.server_id
		ORDER BY c.name LIMIT 5000`)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Export failed.")
		return
	}
	defer rows.Close()
	headers := []string{"id", "server", "name", "image", "state", "health", "restarts", "compose_project"}
	writeExport(w, format, "containers", headers, rowsScanAll(rows, len(headers)))
}

func (s *Server) exportAlerts(w http.ResponseWriter, r *http.Request, format string) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT id::text, severity, status, COALESCE(server_id::text,''), message, first_seen_at::text, last_seen_at::text
		FROM alert_instances ORDER BY last_seen_at DESC LIMIT 5000`)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Export failed.")
		return
	}
	defer rows.Close()
	headers := []string{"id", "severity", "status", "server_id", "message", "first_seen_at", "last_seen_at"}
	writeExport(w, format, "alerts", headers, rowsScanAll(rows, len(headers)))
}

func (s *Server) exportEvents(w http.ResponseWriter, r *http.Request, format string) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT id::text, ts::text, kind, severity, COALESCE(server_id::text,''), message
		FROM infrastructure_events ORDER BY ts DESC LIMIT 5000`)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Export failed.")
		return
	}
	defer rows.Close()
	headers := []string{"id", "ts", "kind", "severity", "server_id", "message"}
	writeExport(w, format, "events", headers, rowsScanAll(rows, len(headers)))
}

type scannable interface {
	Next() bool
	Scan(dest ...any) error
}

func rowsScanAll(rows scannable, n int) [][]string {
	out := make([][]string, 0)
	for rows.Next() {
		vals := make([]any, n)
		ptrs := make([]any, n)
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			continue
		}
		line := make([]string, n)
		for i, v := range vals {
			switch t := v.(type) {
			case string:
				line[i] = t
			case []byte:
				line[i] = string(t)
			case nil:
				line[i] = ""
			default:
				line[i] = fmt.Sprint(t)
			}
		}
		out = append(out, line)
	}
	return out
}

func writeExport(w http.ResponseWriter, format, name string, headers []string, rows [][]string) {
	filename := fmt.Sprintf("fleetdeck-%s-%s.%s", name, time.Now().UTC().Format("20060102T150405Z"), format)
	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename="+filename)
		cw := csv.NewWriter(w)
		_ = cw.Write(headers)
		_ = cw.WriteAll(rows)
		cw.Flush()
		return
	}
	data := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		m := map[string]string{}
		for i, h := range headers {
			if i < len(row) {
				m[h] = row[i]
			}
		}
		data = append(data, m)
	}
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	httpx.JSON(w, http.StatusOK, map[string]any{"kind": name, "exported_at": time.Now().UTC(), "data": data})
}

func (s *Server) handleSelfMetrics(w http.ResponseWriter, r *http.Request) {
	var servers, online, agentsOnline, activeAlerts, rawPoints int
	_ = s.pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM servers`).Scan(&servers)
	_ = s.pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM servers WHERE status='online'`).Scan(&online)
	_ = s.pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM agents WHERE status='online'`).Scan(&agentsOnline)
	_ = s.pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM alert_instances WHERE status IN ('active','acknowledged')`).Scan(&activeAlerts)
	_ = s.pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM server_metrics_raw WHERE ts > now() - interval '1 hour'`).Scan(&rawPoints)

	ws := worker.Snapshot()
	workerOut := map[string]any{}
	if !ws.LastRetainAt.IsZero() {
		workerOut["last_retain_at"] = ws.LastRetainAt
	}
	if !ws.LastAlertsAt.IsZero() {
		workerOut["last_alerts_at"] = ws.LastAlertsAt
	}
	if !ws.LastOfflineAt.IsZero() {
		workerOut["last_offline_at"] = ws.LastOfflineAt
	}
	if !ws.LastPartAt.IsZero() {
		workerOut["last_partitions_at"] = ws.LastPartAt
	}

	started := time.Now().UTC()
	httpx.JSON(w, http.StatusOK, map[string]any{
		"api": map[string]any{
			"status":         "ok",
			"started_at":     processStartedAt,
			"uptime_seconds": int(time.Since(processStartedAt).Seconds()),
			"checked_at":     started,
		},
		"fleet": map[string]any{
			"servers":        servers,
			"online_servers": online,
			"online_agents":  agentsOnline,
			"active_alerts":  activeAlerts,
		},
		"ingestion": map[string]any{
			"server_metric_points_last_hour": rawPoints,
		},
		"worker": workerOut,
		"webhook_configured": strings.TrimSpace(s.cfg.AlertWebhookURL) != "" || s.alertsWebhookConfigured(r.Context()),
	})
}

var processStartedAt = time.Now().UTC()

func (s *Server) alertsWebhookConfigured(ctx context.Context) bool {
	if strings.TrimSpace(s.cfg.AlertWebhookURL) != "" {
		return true
	}
	var rawJSON []byte
	if err := s.pool.QueryRow(ctx, `SELECT value FROM settings WHERE key='alerts'`).Scan(&rawJSON); err != nil {
		return false
	}
	var m struct {
		WebhookURL string `json:"webhook_url"`
	}
	if json.Unmarshal(rawJSON, &m) != nil {
		return false
	}
	return strings.TrimSpace(m.WebhookURL) != ""
}

func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(ctxUser).(auth.User)
	backup := map[string]any{
		"version":     1,
		"exported_at": time.Now().UTC(),
		"warning":     "Does not include historical raw metrics by default.",
	}

	settings := map[string]json.RawMessage{}
	srows, err := s.pool.Query(r.Context(), `SELECT key, value FROM settings`)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Backup failed.")
		return
	}
	for srows.Next() {
		var k string
		var v json.RawMessage
		_ = srows.Scan(&k, &v)
		settings[k] = v
	}
	srows.Close()
	backup["settings"] = settings

	servers := []map[string]any{}
	rows, err := s.pool.Query(r.Context(), `
		SELECT id, name, hostname, primary_address, notes, maintenance, labels FROM servers ORDER BY name`)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Backup failed.")
		return
	}
	for rows.Next() {
		var id uuid.UUID
		var name, hostname, addr, notes string
		var maint bool
		var labels json.RawMessage
		_ = rows.Scan(&id, &name, &hostname, &addr, &notes, &maint, &labels)
		servers = append(servers, map[string]any{
			"id": id, "name": name, "hostname": hostname, "primary_address": addr,
			"notes": notes, "maintenance": maint, "labels": labels,
		})
	}
	rows.Close()
	backup["servers"] = servers

	rules := []map[string]any{}
	rrows, err := s.pool.Query(r.Context(), `
		SELECT id, name, enabled, severity, scope_type, scope_ids, metric, operator, threshold, duration_seconds, cooldown_seconds
		FROM alert_rules ORDER BY name`)
	if err == nil {
		for rrows.Next() {
			var id uuid.UUID
			var name, severity, scopeType, metric, operator string
			var enabled bool
			var scopeIDs json.RawMessage
			var threshold float64
			var duration, cooldown int
			_ = rrows.Scan(&id, &name, &enabled, &severity, &scopeType, &scopeIDs, &metric, &operator, &threshold, &duration, &cooldown)
			rules = append(rules, map[string]any{
				"id": id, "name": name, "enabled": enabled, "severity": severity, "scope_type": scopeType,
				"scope_ids": scopeIDs, "metric": metric, "operator": operator, "threshold": threshold,
				"duration_seconds": duration, "cooldown_seconds": cooldown,
			})
		}
		rrows.Close()
	}
	backup["alert_rules"] = rules

	layouts := []map[string]any{}
	lrows, _ := s.pool.Query(r.Context(), `SELECT id, user_id, name, layout, is_default FROM dashboard_layouts`)
	if lrows != nil {
		for lrows.Next() {
			var id, userID uuid.UUID
			var name string
			var layout json.RawMessage
			var def bool
			_ = lrows.Scan(&id, &userID, &name, &layout, &def)
			layouts = append(layouts, map[string]any{"id": id, "user_id": userID, "name": name, "layout": layout, "is_default": def})
		}
		lrows.Close()
	}
	backup["dashboard_layouts"] = layouts

	uid := u.ID
	s.auth.Audit(r.Context(), &uid, "backup.create", "backup", "", "ok", r.RemoteAddr, nil)
	w.Header().Set("Content-Disposition", "attachment; filename=fleetdeck-backup-"+time.Now().UTC().Format("20060102T150405Z")+".json")
	httpx.JSON(w, http.StatusOK, backup)
}

func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(ctxUser).(auth.User)
	var body struct {
		Version    int                       `json:"version"`
		Confirm    bool                      `json:"confirm"`
		Mode       string                    `json:"mode"` // merge (default) | replace_settings
		Settings   map[string]json.RawMessage `json:"settings"`
		AlertRules []struct {
			Name             string  `json:"name"`
			Enabled          bool    `json:"enabled"`
			Severity         string  `json:"severity"`
			Metric           string  `json:"metric"`
			Operator         string  `json:"operator"`
			Threshold        float64 `json:"threshold"`
			DurationSeconds  int     `json:"duration_seconds"`
			CooldownSeconds  int     `json:"cooldown_seconds"`
		} `json:"alert_rules"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "Invalid backup payload.")
		return
	}
	if !body.Confirm {
		httpx.Error(w, http.StatusBadRequest, "confirmation_required",
			"Restore requires confirm=true. Existing data will not be overwritten silently.")
		return
	}
	if body.Version != 0 && body.Version != 1 {
		httpx.Error(w, http.StatusBadRequest, "unsupported_version", "Unsupported backup version.")
		return
	}

	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Restore failed.")
		return
	}
	defer tx.Rollback(r.Context())

	restoredSettings := 0
	for k, v := range body.Settings {
		if k != "general" && k != "metrics" && k != "alerts" {
			continue
		}
		_, err := tx.Exec(r.Context(), `
			INSERT INTO settings(key, value, updated_at) VALUES ($1,$2::jsonb,now())
			ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()`, k, string(v))
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Restore settings failed.")
			return
		}
		restoredSettings++
	}

	restoredRules := 0
	for _, rule := range body.AlertRules {
		if rule.Name == "" || rule.Metric == "" {
			continue
		}
		sev := rule.Severity
		if sev == "" {
			sev = "warning"
		}
		_, err := tx.Exec(r.Context(), `
			INSERT INTO alert_rules (name, enabled, severity, scope_type, metric, operator, threshold, duration_seconds, cooldown_seconds)
			VALUES ($1,$2,$3,'all',$4,$5,$6,NULLIF($7,0),NULLIF($8,0))`,
			rule.Name, rule.Enabled, sev, rule.Metric, rule.Operator, rule.Threshold,
			rule.DurationSeconds, rule.CooldownSeconds)
		if err != nil {
			continue
		}
		restoredRules++
	}

	if err := tx.Commit(r.Context()); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Restore commit failed.")
		return
	}
	uid := u.ID
	s.auth.Audit(r.Context(), &uid, "backup.restore", "backup", "", "ok", r.RemoteAddr, map[string]any{
		"settings": restoredSettings, "alert_rules": restoredRules,
	})
	httpx.JSON(w, http.StatusOK, map[string]any{
		"ok":                true,
		"settings_restored": restoredSettings,
		"alert_rules_added": restoredRules,
		"note":              "Servers/agents were not auto-recreated (re-enroll required). No silent overwrite of live fleet state.",
	})
}

type notificationNote struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	Href      string    `json:"href"`
}

func (s *Server) collectNotifications(r *http.Request) []notificationNote {
	out := make([]notificationNote, 0)
	rows, err := s.pool.Query(r.Context(), `
		SELECT id::text, severity, message, last_seen_at
		FROM alert_instances
		WHERE status IN ('active','acknowledged')
		ORDER BY last_seen_at DESC LIMIT 20`)
	if err == nil {
		for rows.Next() {
			var id, sev, msg string
			var ts time.Time
			_ = rows.Scan(&id, &sev, &msg, &ts)
			out = append(out, notificationNote{
				ID: "alert-" + id, Kind: "alert", Title: strings.ToUpper(sev) + " alert",
				Body: msg, CreatedAt: ts, Href: "/alerts",
			})
		}
		rows.Close()
	}
	erows, err := s.pool.Query(r.Context(), `
		SELECT id::text, kind, message, ts FROM infrastructure_events
		WHERE severity IN ('warning','critical') OR kind LIKE 'agent.%' OR kind LIKE 'servers.%'
		ORDER BY ts DESC LIMIT 20`)
	if err == nil {
		for erows.Next() {
			var id, kind, msg string
			var ts time.Time
			_ = erows.Scan(&id, &kind, &msg, &ts)
			out = append(out, notificationNote{
				ID: "event-" + id, Kind: "event", Title: kind, Body: msg, CreatedAt: ts, Href: "/events",
			})
		}
		erows.Close()
	}
	return out
}

func (s *Server) dismissedNotificationIDs(r *http.Request, userID uuid.UUID) map[string]struct{} {
	dismissed := map[string]struct{}{}
	drows, err := s.pool.Query(r.Context(), `
		SELECT notification_id FROM notification_dismissals WHERE user_id=$1`, userID)
	if err != nil {
		return dismissed
	}
	defer drows.Close()
	for drows.Next() {
		var nid string
		if err := drows.Scan(&nid); err == nil && nid != "" {
			dismissed[nid] = struct{}{}
		}
	}
	return dismissed
}

func validNotificationID(id string) bool {
	if id == "" || len(id) > 200 {
		return false
	}
	if strings.HasPrefix(id, "alert-") || strings.HasPrefix(id, "event-") {
		return true
	}
	return false
}

func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	// In-app notifications derived from recent critical alerts + events (no external integrations required).
	u := r.Context().Value(ctxUser).(auth.User)
	dismissed := s.dismissedNotificationIDs(r, u.ID)
	raw := s.collectNotifications(r)
	out := make([]notificationNote, 0, len(raw))
	for _, n := range raw {
		if _, ok := dismissed[n.ID]; ok {
			continue
		}
		out = append(out, n)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"data": out,
		"integrations": map[string]any{
			"email": false, "discord": false, "slack": false,
			"webhooks": s.alertsWebhookConfigured(r.Context()),
			"push":     false,
			"note":     "Set ALERT_WEBHOOK_URL or settings.alerts.webhook_url for fire/resolve POSTs. Email/Discord/Slack remain optional.",
		},
	})
}

func (s *Server) handleDismissNotification(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(ctxUser).(auth.User)
	id := chi.URLParam(r, "id")
	if !validNotificationID(id) {
		httpx.Error(w, http.StatusBadRequest, "validation", "Invalid notification id.")
		return
	}
	_, err := s.pool.Exec(r.Context(), `
		INSERT INTO notification_dismissals (user_id, notification_id)
		VALUES ($1, $2)
		ON CONFLICT (user_id, notification_id) DO NOTHING`, u.ID, id)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not dismiss notification.")
		return
	}
	uid := u.ID
	s.auth.Audit(r.Context(), &uid, "notification.dismiss", "notification", id, "ok", r.RemoteAddr, nil)
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

func (s *Server) handleClearNotifications(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(ctxUser).(auth.User)
	notes := s.collectNotifications(r)
	cleared := 0
	for _, n := range notes {
		tag, err := s.pool.Exec(r.Context(), `
			INSERT INTO notification_dismissals (user_id, notification_id)
			VALUES ($1, $2)
			ON CONFLICT (user_id, notification_id) DO NOTHING`, u.ID, n.ID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not clear notifications.")
			return
		}
		cleared += int(tag.RowsAffected())
	}
	uid := u.ID
	s.auth.Audit(r.Context(), &uid, "notification.clear", "notification", "", "ok", r.RemoteAddr, map[string]any{
		"cleared": cleared, "visible": len(notes),
	})
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true, "cleared": cleared})
}
