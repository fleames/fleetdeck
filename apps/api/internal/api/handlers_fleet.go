package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/fleetdeck/fleetdeck/apps/api/internal/auth"
	"github.com/fleetdeck/fleetdeck/apps/api/internal/httpx"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type latestMetrics struct {
	CPUPct         *float64   `json:"cpu_pct"`
	MemUsedBytes   *int64     `json:"mem_used_bytes"`
	MemTotalApprox *int64     `json:"mem_total_bytes"`
	DiskUsedBytes  *int64     `json:"disk_used_bytes"`
	DiskTotalBytes *int64     `json:"disk_total_bytes"`
	NetRxBps       *int64     `json:"net_rx_bps"`
	NetTxBps       *int64     `json:"net_tx_bps"`
	UptimeSeconds  *int64     `json:"uptime_seconds"`
	LastUpdated    *time.Time `json:"last_updated"`
}

type serverRow struct {
	ID              uuid.UUID      `json:"id"`
	Name            string         `json:"name"`
	Hostname        string         `json:"hostname"`
	PrimaryAddress  string         `json:"primary_address"`
	OSName          string         `json:"os_name"`
	OSVersion       string         `json:"os_version"`
	Arch            string         `json:"arch"`
	Status          string         `json:"status"`
	HealthState     string         `json:"health_state"`
	DockerAvailable bool           `json:"docker_available"`
	LastSeenAt      *time.Time     `json:"last_seen_at"`
	LastMetricsAt   *time.Time     `json:"last_metrics_at"`
	Maintenance     bool           `json:"maintenance"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	AgentVersion    *string        `json:"agent_version,omitempty"`
	Metrics         *latestMetrics `json:"metrics,omitempty"`
	RunningContainers int          `json:"running_containers"`
}

func (s *Server) handleListServers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT s.id, s.name, s.hostname, s.primary_address, s.os_name, s.os_version, s.arch,
		       s.status, s.health_state, s.docker_available, s.last_seen_at, s.last_metrics_at,
		       s.maintenance, s.created_at, s.updated_at, a.agent_version,
		       m.cpu_pct, m.mem_used_bytes,
		       CASE WHEN m.mem_used_bytes IS NOT NULL AND m.mem_available_bytes IS NOT NULL
		            THEN m.mem_used_bytes + m.mem_available_bytes ELSE NULL END AS mem_total,
		       m.disk_used_bytes, m.disk_total_bytes, m.net_rx_bps, m.net_tx_bps, m.uptime_seconds, m.ts,
		       (SELECT COUNT(*) FROM containers c WHERE c.server_id=s.id AND c.state='running') AS running_containers
		FROM servers s
		LEFT JOIN agents a ON a.server_id = s.id
		LEFT JOIN LATERAL (
			SELECT * FROM server_metrics_raw sm
			WHERE sm.server_id = s.id
			ORDER BY sm.ts DESC
			LIMIT 1
		) m ON true
		ORDER BY s.name ASC`)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list servers.")
		return
	}
	defer rows.Close()

	out := make([]serverRow, 0)
	for rows.Next() {
		var row serverRow
		var m latestMetrics
		if err := rows.Scan(
			&row.ID, &row.Name, &row.Hostname, &row.PrimaryAddress, &row.OSName, &row.OSVersion, &row.Arch,
			&row.Status, &row.HealthState, &row.DockerAvailable, &row.LastSeenAt, &row.LastMetricsAt,
			&row.Maintenance, &row.CreatedAt, &row.UpdatedAt, &row.AgentVersion,
			&m.CPUPct, &m.MemUsedBytes, &m.MemTotalApprox, &m.DiskUsedBytes, &m.DiskTotalBytes,
			&m.NetRxBps, &m.NetTxBps, &m.UptimeSeconds, &m.LastUpdated, &row.RunningContainers,
		); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list servers.")
			return
		}
		if m.LastUpdated != nil {
			row.Metrics = &m
		}
		out = append(out, row)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"data": out,
		"meta": map[string]any{
			"total":                 len(out),
			"current_agent_version": s.cfg.AgentVersion,
		},
	})
}

func (s *Server) handleGetServer(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation", "Invalid server id.")
		return
	}
	var row serverRow
	var m latestMetrics
	err = s.pool.QueryRow(r.Context(), `
		SELECT s.id, s.name, s.hostname, s.primary_address, s.os_name, s.os_version, s.arch,
		       s.status, s.health_state, s.docker_available, s.last_seen_at, s.last_metrics_at,
		       s.maintenance, s.created_at, s.updated_at, a.agent_version,
		       m.cpu_pct, m.mem_used_bytes,
		       CASE WHEN m.mem_used_bytes IS NOT NULL AND m.mem_available_bytes IS NOT NULL
		            THEN m.mem_used_bytes + m.mem_available_bytes ELSE NULL END AS mem_total,
		       m.disk_used_bytes, m.disk_total_bytes, m.net_rx_bps, m.net_tx_bps, m.uptime_seconds, m.ts,
		       (SELECT COUNT(*) FROM containers c WHERE c.server_id=s.id AND c.state='running') AS running_containers
		FROM servers s
		LEFT JOIN agents a ON a.server_id = s.id
		LEFT JOIN LATERAL (
			SELECT * FROM server_metrics_raw sm
			WHERE sm.server_id = s.id
			ORDER BY sm.ts DESC
			LIMIT 1
		) m ON true
		WHERE s.id=$1`, id,
	).Scan(
		&row.ID, &row.Name, &row.Hostname, &row.PrimaryAddress, &row.OSName, &row.OSVersion, &row.Arch,
		&row.Status, &row.HealthState, &row.DockerAvailable, &row.LastSeenAt, &row.LastMetricsAt,
		&row.Maintenance, &row.CreatedAt, &row.UpdatedAt, &row.AgentVersion,
		&m.CPUPct, &m.MemUsedBytes, &m.MemTotalApprox, &m.DiskUsedBytes, &m.DiskTotalBytes,
		&m.NetRxBps, &m.NetTxBps, &m.UptimeSeconds, &m.LastUpdated, &row.RunningContainers,
	)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "Server not found.")
		return
	}
	if m.LastUpdated != nil {
		row.Metrics = &m
	}
	type serverDetail struct {
		serverRow
		CurrentAgentVersion string `json:"current_agent_version"`
	}
	httpx.JSON(w, http.StatusOK, serverDetail{serverRow: row, CurrentAgentVersion: s.cfg.AgentVersion})
}

func (s *Server) handleServerLatestMetrics(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation", "Invalid server id.")
		return
	}
	var m latestMetrics
	err = s.pool.QueryRow(r.Context(), `
		SELECT cpu_pct, mem_used_bytes,
		       CASE WHEN mem_used_bytes IS NOT NULL AND mem_available_bytes IS NOT NULL
		            THEN mem_used_bytes + mem_available_bytes ELSE NULL END,
		       disk_used_bytes, disk_total_bytes, net_rx_bps, net_tx_bps, uptime_seconds, ts
		FROM server_metrics_raw WHERE server_id=$1 ORDER BY ts DESC LIMIT 1`, id,
	).Scan(&m.CPUPct, &m.MemUsedBytes, &m.MemTotalApprox, &m.DiskUsedBytes, &m.DiskTotalBytes,
		&m.NetRxBps, &m.NetTxBps, &m.UptimeSeconds, &m.LastUpdated)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "metrics_unavailable", "No metrics have been reported for this server yet.")
		return
	}
	httpx.JSON(w, http.StatusOK, m)
}

func (s *Server) handleServerMetricsHistory(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation", "Invalid server id.")
		return
	}
	rangeKey := r.URL.Query().Get("range")
	if rangeKey == "" {
		rangeKey = "1h"
	}
	requested := map[string]time.Duration{
		"15m": 15 * time.Minute,
		"1h":  time.Hour,
		"6h":  6 * time.Hour,
		"24h": 24 * time.Hour,
		"7d":  7 * 24 * time.Hour,
		"30d": 30 * 24 * time.Hour,
	}[rangeKey]
	if requested == 0 {
		requested = time.Hour
		rangeKey = "1h"
	}

	source, maxRetain := metricsHistorySource(rangeKey, s.cfg.RawRetentionDays, s.cfg.Agg5mRetentionDays, s.cfg.Agg1hRetentionDays)
	dur := requested
	truncated := false
	if maxRetain > 0 && dur > maxRetain {
		dur = maxRetain
		truncated = true
	}
	since := time.Now().UTC().Add(-dur)

	type point struct {
		TS             time.Time `json:"ts"`
		CPUPct         *float64  `json:"cpu_pct"`
		MemUsedBytes   *int64    `json:"mem_used_bytes"`
		DiskUsedBytes  *int64    `json:"disk_used_bytes"`
		DiskTotalBytes *int64    `json:"disk_total_bytes"`
		NetRxBps       *int64    `json:"net_rx_bps"`
		NetTxBps       *int64    `json:"net_tx_bps"`
	}
	out := make([]point, 0)

	switch source {
	case "5m", "1h":
		var rows pgx.Rows
		var err error
		if source == "1h" {
			rows, err = s.pool.Query(r.Context(), `
				SELECT bucket, cpu_pct_avg, mem_used_bytes_avg, disk_used_bytes_avg, net_rx_bps_avg, net_tx_bps_avg
				FROM server_metrics_1h
				WHERE server_id=$1 AND bucket >= $2
				ORDER BY bucket ASC
				LIMIT 5000`, id, since)
		} else {
			rows, err = s.pool.Query(r.Context(), `
				SELECT bucket, cpu_pct_avg, mem_used_bytes_avg, disk_used_bytes_avg, net_rx_bps_avg, net_tx_bps_avg
				FROM server_metrics_5m
				WHERE server_id=$1 AND bucket >= $2
				ORDER BY bucket ASC
				LIMIT 5000`, id, since)
		}
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not load metrics history.")
			return
		}
		defer rows.Close()
		for rows.Next() {
			var p point
			if err := rows.Scan(&p.TS, &p.CPUPct, &p.MemUsedBytes, &p.DiskUsedBytes, &p.NetRxBps, &p.NetTxBps); err != nil {
				httpx.Error(w, http.StatusInternalServerError, "internal", "Could not load metrics history.")
				return
			}
			out = append(out, p)
		}
	default:
		rows, err := s.pool.Query(r.Context(), `
			SELECT ts, cpu_pct, mem_used_bytes, disk_used_bytes, disk_total_bytes, net_rx_bps, net_tx_bps
			FROM server_metrics_raw
			WHERE server_id=$1 AND ts >= $2
			ORDER BY ts ASC
			LIMIT 5000`, id, since)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not load metrics history.")
			return
		}
		defer rows.Close()
		for rows.Next() {
			var p point
			if err := rows.Scan(&p.TS, &p.CPUPct, &p.MemUsedBytes, &p.DiskUsedBytes, &p.DiskTotalBytes, &p.NetRxBps, &p.NetTxBps); err != nil {
				httpx.Error(w, http.StatusInternalServerError, "internal", "Could not load metrics history.")
				return
			}
			out = append(out, p)
		}
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"data":      out,
		"range":     rangeKey,
		"since":     since,
		"source":    source,
		"truncated": truncated,
	})
}

// metricsHistorySource picks raw vs rollup tables and the max lookback for that store.
func metricsHistorySource(rangeKey string, rawDays, agg5mDays, agg1hDays int) (source string, maxRetain time.Duration) {
	rawMax := time.Duration(rawDays) * 24 * time.Hour
	agg5mMax := time.Duration(agg5mDays) * 24 * time.Hour
	agg1hMax := time.Duration(agg1hDays) * 24 * time.Hour
	switch rangeKey {
	case "24h", "7d":
		return "5m", agg5mMax
	case "30d":
		return "1h", agg1hMax
	default:
		return "raw", rawMax
	}
}

func (s *Server) handleCreateServer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "Invalid request body.")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		httpx.Error(w, http.StatusBadRequest, "validation", "Server name is required.")
		return
	}

	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create server.")
		return
	}
	defer tx.Rollback(r.Context())

	var id uuid.UUID
	err = tx.QueryRow(r.Context(), `
		INSERT INTO servers (name, status, health_state)
		VALUES ($1, 'pending', 'unknown')
		RETURNING id`, body.Name,
	).Scan(&id)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create server.")
		return
	}
	_, err = tx.Exec(r.Context(), `
		INSERT INTO agents (server_id, status) VALUES ($1, 'pending')`, id)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create agent registry row.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create server.")
		return
	}

	u := r.Context().Value(ctxUser).(auth.User)
	uid := u.ID
	s.auth.Audit(r.Context(), &uid, "server.create", "server", id.String(), "ok", r.RemoteAddr, map[string]any{"name": body.Name})
	httpx.JSON(w, http.StatusCreated, map[string]any{"id": id, "name": body.Name, "status": "pending"})
}

// handleRemoveServer uninstalls the VPS agent (when online) then hard-deletes the server row
// (CASCADE removes agent, credentials, commands, docker inventory, metrics).
func (s *Server) handleRemoveServer(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(ctxUser).(auth.User)
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation", "Invalid server id.")
		return
	}
	var body struct {
		Confirm bool `json:"confirm"`
		Force   bool `json:"force"`
	}
	if err := httpx.Decode(r, &body); err != nil || !body.Confirm {
		httpx.Error(w, http.StatusBadRequest, "confirmation_required",
			"Removing a server requires confirm=true. Use force=true to purge the panel without waiting for agent uninstall.")
		return
	}

	var name, serverStatus, agentStatus string
	var agentID *uuid.UUID
	var lastHeartbeat *time.Time
	err = s.pool.QueryRow(r.Context(), `
		SELECT s.name, s.status, a.id, COALESCE(a.status, 'pending'), a.last_heartbeat_at
		FROM servers s
		LEFT JOIN agents a ON a.server_id = s.id
		WHERE s.id=$1`, id,
	).Scan(&name, &serverStatus, &agentID, &agentStatus, &lastHeartbeat)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "Server not found.")
		return
	}

	agentOnline := agentID != nil && agentReachable(agentStatus, lastHeartbeat)
	uid := u.ID

	if agentOnline && !body.Force {
		payload, _ := json.Marshal(map[string]any{"reason": "panel_remove"})
		var cmdID uuid.UUID
		err = s.pool.QueryRow(r.Context(), `
			INSERT INTO agent_commands (agent_id, server_id, type, payload, expires_at)
			VALUES ($1,$2,'agent.uninstall',$3::jsonb, now() + interval '90 seconds')
			RETURNING id`, *agentID, id, string(payload),
		).Scan(&cmdID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not enqueue agent uninstall.")
			return
		}
		s.auth.Audit(r.Context(), &uid, "server.remove", "server", id.String(), "uninstall_queued", r.RemoteAddr, map[string]any{
			"name": name, "command_id": cmdID,
		})
		_, _ = s.pool.Exec(r.Context(), `
			INSERT INTO infrastructure_events (kind, severity, server_id, message, context)
			VALUES ('agent.uninstall','info',$1,$2,$3::jsonb)`,
			id, "Queued agent uninstall for "+name,
			string(mustJSON(map[string]any{"command_id": cmdID, "user": u.Email})),
		)

		status, result, errText, timedOut := s.waitCommand(r, cmdID, 45*time.Second)
		if timedOut {
			httpx.ErrorDetails(w, http.StatusGatewayTimeout, "uninstall_timeout",
				"Agent did not finish uninstall in time. Retry, or remove with force=true to delete panel records only.",
				map[string]any{"command_id": cmdID, "server_id": id, "force_allowed": true}, nil)
			return
		}
		if status != "completed" {
			msg := "Agent uninstall failed."
			if errText != "" {
				msg = errText
			}
			s.auth.Audit(r.Context(), &uid, "server.remove", "server", id.String(), "uninstall_failed", r.RemoteAddr, map[string]any{
				"error": msg, "command_id": cmdID,
			})
			httpx.ErrorDetails(w, http.StatusBadGateway, "uninstall_failed",
				msg+" Use force=true to remove this server from the panel anyway.",
				map[string]any{"command_id": cmdID, "result": result, "force_allowed": true}, nil)
			return
		}
	} else if !agentOnline && !body.Force {
		// Never enrolled / offline: require explicit force so UI copy is clear.
		if agentStatus == "pending" || serverStatus == "pending" {
			// Pending (no live agent): allow panel-only delete without force.
		} else {
			httpx.ErrorDetails(w, http.StatusConflict, "agent_offline",
				"Agent appears offline. Confirm force=true to remove this server from the panel only (VPS agent will not be uninstalled).",
				map[string]any{"server_id": id, "agent_status": agentStatus, "force_required": true}, nil)
			return
		}
	}

	// Explicit inventory purge before server delete (CASCADE also covers these; this
	// guarantees UI categories clear even if an older DB drifted off CASCADE FKs).
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not delete server.")
		return
	}
	defer tx.Rollback(r.Context())

	for _, q := range []string{
		`DELETE FROM container_metrics_raw WHERE server_id=$1`,
		`DELETE FROM server_metrics_raw WHERE server_id=$1`,
		`DELETE FROM server_metrics_5m WHERE server_id=$1`,
		`DELETE FROM server_metrics_1h WHERE server_id=$1`,
		`DELETE FROM agent_commands WHERE server_id=$1`,
		`DELETE FROM containers WHERE server_id=$1`,
		`DELETE FROM images WHERE server_id=$1`,
		`DELETE FROM volumes WHERE server_id=$1`,
		`DELETE FROM networks WHERE server_id=$1`,
		`DELETE FROM compose_projects WHERE server_id=$1`,
		`DELETE FROM docker_hosts WHERE server_id=$1`,
	} {
		if _, err := tx.Exec(r.Context(), q, id); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not purge server inventory.")
			return
		}
	}
	// agents (+ credentials) CASCADE from servers; delete server last.
	tag, err := tx.Exec(r.Context(), `DELETE FROM servers WHERE id=$1`, id)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not delete server.")
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Error(w, http.StatusNotFound, "not_found", "Server not found.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not delete server.")
		return
	}

	mode := "uninstalled"
	if body.Force {
		mode = "force"
	} else if !agentOnline {
		mode = "panel_only"
	}
	s.auth.Audit(r.Context(), &uid, "server.remove", "server", id.String(), "ok", r.RemoteAddr, map[string]any{
		"name": name, "mode": mode,
	})
	_, _ = s.pool.Exec(r.Context(), `
		INSERT INTO infrastructure_events (kind, severity, message, context)
		VALUES ('server.removed','info',$1,$2::jsonb)`,
		"Removed server "+name+" from panel ("+mode+").",
		string(mustJSON(map[string]any{"server_id": id, "user": u.Email, "mode": mode})),
	)
	if s.hub != nil {
		s.hub.Broadcast("servers.updated", map[string]any{"server_id": id, "removed": true})
		s.hub.Broadcast("docker.updated", map[string]any{"server_id": id, "removed": true})
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"ok": true, "id": id, "name": name, "mode": mode,
	})
}

// handleUpdateAgent queues agent.update so an online agent pulls a new binary from CDN and restarts.
func (s *Server) handleUpdateAgent(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(ctxUser).(auth.User)
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation", "Invalid server id.")
		return
	}
	var body struct {
		Confirm bool   `json:"confirm"`
		Channel string `json:"channel"`
	}
	if err := httpx.Decode(r, &body); err != nil || !body.Confirm {
		httpx.Error(w, http.StatusBadRequest, "confirmation_required",
			"Updating an agent requires confirm=true.")
		return
	}

	var name, agentStatus string
	var agentID *uuid.UUID
	var lastHeartbeat *time.Time
	var agentVersion string
	err = s.pool.QueryRow(r.Context(), `
		SELECT s.name, a.id, COALESCE(a.status, 'pending'), a.last_heartbeat_at, COALESCE(a.agent_version, '')
		FROM servers s
		LEFT JOIN agents a ON a.server_id = s.id
		WHERE s.id=$1`, id,
	).Scan(&name, &agentID, &agentStatus, &lastHeartbeat, &agentVersion)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "Server not found.")
		return
	}
	if agentID == nil {
		httpx.Error(w, http.StatusConflict, "not_enrolled",
			"No agent enrolled for this server. Install the agent first.")
		return
	}

	agentOnline := agentReachable(agentStatus, lastHeartbeat)
	if !agentOnline {
		httpx.ErrorDetails(w, http.StatusConflict, "agent_offline",
			"Agent appears offline. Wait until it is online, then retry Update agent. There is no offline update path.",
			map[string]any{"server_id": id, "agent_status": agentStatus, "wait_required": true}, nil)
		return
	}

	cdnBase := strings.TrimRight(s.cfg.AgentCDNBase, "/")
	if cdnBase == "" {
		cdnBase = "https://cdn.tarkovbot.com/fleetdeck"
	}
	channel := strings.TrimSpace(body.Channel)
	if channel == "" {
		channel = s.cfg.AgentCDNChannel
	}
	if channel == "" {
		channel = "latest"
	}

	payload, _ := json.Marshal(map[string]any{
		"cdn_base": cdnBase,
		"channel":  channel,
		"url":      cdnBase + "/" + channel,
	})
	var cmdID uuid.UUID
	err = s.pool.QueryRow(r.Context(), `
		INSERT INTO agent_commands (agent_id, server_id, type, payload, expires_at)
		VALUES ($1,$2,'agent.update',$3::jsonb, now() + interval '3 minutes')
		RETURNING id`, *agentID, id, string(payload),
	).Scan(&cmdID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not enqueue agent update.")
		return
	}

	uid := u.ID
	s.auth.Audit(r.Context(), &uid, "agent.update", "server", id.String(), "queued", r.RemoteAddr, map[string]any{
		"name": name, "command_id": cmdID, "cdn_base": cdnBase, "channel": channel,
	})
	_, _ = s.pool.Exec(r.Context(), `
		INSERT INTO infrastructure_events (kind, severity, server_id, message, context)
		VALUES ('agent.update','info',$1,$2,$3::jsonb)`,
		id, "Queued agent update for "+name+" ("+channel+")",
		string(mustJSON(map[string]any{"command_id": cmdID, "user": u.Email, "channel": channel, "cdn_base": cdnBase})),
	)

	status, result, errText, timedOut := s.waitCommand(r, cmdID, 90*time.Second)
	if timedOut {
		httpx.ErrorDetails(w, http.StatusGatewayTimeout, "update_timeout",
			"Agent did not finish staging the update in time. Check agent logs, or retry when the host is responsive.",
			map[string]any{"command_id": cmdID, "server_id": id, "cdn": cdnBase + "/" + channel}, nil)
		return
	}
	if status != "completed" {
		msg := "Agent update failed."
		if errText != "" {
			msg = errText
		}
		s.auth.Audit(r.Context(), &uid, "agent.update", "server", id.String(), "failed", r.RemoteAddr, map[string]any{
			"error": msg, "command_id": cmdID,
		})
		httpx.ErrorDetails(w, http.StatusBadGateway, "update_failed", msg,
			map[string]any{"command_id": cmdID, "result": result}, nil)
		return
	}

	s.auth.Audit(r.Context(), &uid, "agent.update", "server", id.String(), "ok", r.RemoteAddr, map[string]any{
		"command_id": cmdID, "previous_version": agentVersion,
	})
	if s.hub != nil {
		s.hub.Broadcast("servers.updated", map[string]any{"server_id": id, "agent_update": true})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"ok":                true,
		"id":                id,
		"name":              name,
		"command_id":        cmdID,
		"result":            result,
		"channel":           channel,
		"cdn_base":          cdnBase,
		"previous_version":  agentVersion,
		"message":           "Update staged; agent is restarting with the new binary. Version refreshes on next heartbeat.",
	})
}

func agentReachable(status string, lastHeartbeat *time.Time) bool {
	if status != "online" {
		return false
	}
	if lastHeartbeat == nil {
		return false
	}
	return time.Since(lastHeartbeat.UTC()) < 2*time.Minute
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT a.id, a.server_id, s.name, a.agent_version, a.os, a.arch, a.status,
		       a.enrolled_at, a.last_heartbeat_at, a.last_latency_ms
		FROM agents a
		JOIN servers s ON s.id = a.server_id
		ORDER BY s.name`)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list agents.")
		return
	}
	defer rows.Close()
	type agentRow struct {
		ID              uuid.UUID  `json:"id"`
		ServerID        uuid.UUID  `json:"server_id"`
		ServerName      string     `json:"server_name"`
		AgentVersion    string     `json:"agent_version"`
		OS              string     `json:"os"`
		Arch            string     `json:"arch"`
		Status          string     `json:"status"`
		EnrolledAt      *time.Time `json:"enrolled_at"`
		LastHeartbeatAt *time.Time `json:"last_heartbeat_at"`
		LastLatencyMs   *int       `json:"last_latency_ms"`
	}
	out := make([]agentRow, 0)
	for rows.Next() {
		var row agentRow
		if err := rows.Scan(&row.ID, &row.ServerID, &row.ServerName, &row.AgentVersion, &row.OS, &row.Arch, &row.Status,
			&row.EnrolledAt, &row.LastHeartbeatAt, &row.LastLatencyMs); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list agents.")
			return
		}
		out = append(out, row)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out, "meta": map[string]any{"total": len(out)}})
}

func (s *Server) handleCreateEnrollmentToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Label      string `json:"label"`
		ServerID   string `json:"server_id"`
		TTLMinutes int    `json:"ttl_minutes"`
		MaxUses    int    `json:"max_uses"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "Invalid request body.")
		return
	}
	if body.TTLMinutes <= 0 {
		body.TTLMinutes = 60
	}
	if body.MaxUses <= 0 {
		body.MaxUses = 1
	}
	serverID, err := uuid.Parse(body.ServerID)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation", "Valid server_id is required.")
		return
	}
	token, err := auth.NewToken(24)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create enrollment token.")
		return
	}
	u := r.Context().Value(ctxUser).(auth.User)
	expires := time.Now().UTC().Add(time.Duration(body.TTLMinutes) * time.Minute)
	var id uuid.UUID
	err = s.pool.QueryRow(r.Context(), `
		INSERT INTO enrollment_tokens (token_hash, label, expires_at, max_uses, created_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`, auth.HashToken(token), body.Label, expires, body.MaxUses, u.ID,
	).Scan(&id)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create enrollment token.")
		return
	}
	// Store intended server binding in label context via settings-like approach: encode in label prefix
	_, _ = s.pool.Exec(r.Context(), `
		UPDATE enrollment_tokens SET label=$1 WHERE id=$2`,
		"server:"+serverID.String()+"|"+body.Label, id,
	)

	uid := u.ID
	s.auth.Audit(r.Context(), &uid, "agent.enrollment_token.create", "server", serverID.String(), "ok", r.RemoteAddr, nil)
	httpx.JSON(w, http.StatusCreated, map[string]any{
		"id":         id,
		"token":      token,
		"server_id":  serverID,
		"expires_at": expires,
		"max_uses":   body.MaxUses,
		"warning":    "Store this token securely. It will not be shown again.",
	})
}

func (s *Server) handleAgentEnroll(w http.ResponseWriter, r *http.Request) {
	if !enrollLimiter.allow(clientIP(r)) {
		httpx.Error(w, http.StatusTooManyRequests, "rate_limited", "Too many enrollment attempts. Try again shortly.")
		return
	}
	var body struct {
		Token        string `json:"token"`
		AgentVersion string `json:"agent_version"`
		Hostname     string `json:"hostname"`
		OS           string `json:"os"`
		Arch         string `json:"arch"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "Invalid request body.")
		return
	}
	if strings.TrimSpace(body.Token) == "" {
		httpx.Error(w, http.StatusBadRequest, "validation", "Enrollment token is required.")
		return
	}

	var tokenID uuid.UUID
	var label string
	var expires time.Time
	var maxUses, uses int
	err := s.pool.QueryRow(r.Context(), `
		SELECT id, label, expires_at, max_uses, uses
		FROM enrollment_tokens
		WHERE token_hash=$1 AND revoked_at IS NULL`, auth.HashToken(body.Token),
	).Scan(&tokenID, &label, &expires, &maxUses, &uses)
	if err != nil || time.Now().UTC().After(expires) || uses >= maxUses {
		httpx.Error(w, http.StatusUnauthorized, "invalid_enrollment_token", "Enrollment token is invalid or expired.")
		return
	}

	serverID, ok := parseServerFromEnrollmentLabel(label)
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid_token_binding", "Enrollment token is not bound to a server.")
		return
	}

	publicID, err := auth.NewToken(16)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not enroll agent.")
		return
	}
	secret, err := auth.NewToken(32)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not enroll agent.")
		return
	}
	secretHash, err := auth.HashSecret(secret)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not enroll agent.")
		return
	}

	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not enroll agent.")
		return
	}
	defer tx.Rollback(r.Context())

	var agentID uuid.UUID
	err = tx.QueryRow(r.Context(), `
		UPDATE agents
		SET agent_version=$2, os=$3, arch=$4, status='online', enrolled_at=now(), last_heartbeat_at=now(), updated_at=now()
		WHERE server_id=$1
		RETURNING id`, serverID, body.AgentVersion, body.OS, body.Arch,
	).Scan(&agentID)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "server_not_found", "Server for this enrollment token was not found.")
		return
	}
	_, err = tx.Exec(r.Context(), `
		UPDATE servers
		SET hostname=COALESCE(NULLIF($2,''), hostname),
		    status='online', health_state='healthy', last_seen_at=now(), updated_at=now()
		WHERE id=$1`, serverID, body.Hostname)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not enroll agent.")
		return
	}
	_, err = tx.Exec(r.Context(), `
		INSERT INTO agent_credentials (agent_id, public_id, secret_hash)
		VALUES ($1, $2, $3)`, agentID, publicID, secretHash)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not enroll agent.")
		return
	}
	_, err = tx.Exec(r.Context(), `
		UPDATE enrollment_tokens SET uses = uses + 1 WHERE id=$1`, tokenID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not enroll agent.")
		return
	}
	_, _ = tx.Exec(r.Context(), `
		INSERT INTO infrastructure_events (kind, severity, server_id, message)
		VALUES ('agent.enrolled', 'info', $1, 'Agent enrolled and connected.')`, serverID)
	if err := tx.Commit(r.Context()); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not enroll agent.")
		return
	}

	httpx.JSON(w, http.StatusCreated, map[string]any{
		"server_id":      serverID,
		"agent_id":       agentID,
		"public_id":      publicID,
		"secret":         secret,
		"api_public_url": s.cfg.APIPublicURL,
		"warning":        "Store agent credentials securely. The secret will not be shown again.",
	})
}

func parseServerFromEnrollmentLabel(label string) (uuid.UUID, bool) {
	if !strings.HasPrefix(label, "server:") {
		return uuid.Nil, false
	}
	rest := strings.TrimPrefix(label, "server:")
	idPart := rest
	if i := strings.IndexByte(rest, '|'); i >= 0 {
		idPart = rest[:i]
	}
	id, err := uuid.Parse(idPart)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

func (s *Server) handleAgentHeartbeat(w http.ResponseWriter, r *http.Request) {
	ident := r.Context().Value(ctxAgent).(agentIdentity)
	var body struct {
		AgentVersion       string   `json:"agent_version"`
		LatencyMS          *int     `json:"latency_ms"`
		ResourceCPUPct     *float64 `json:"resource_cpu_pct"`
		ResourceRSSBytes   *int64   `json:"resource_rss_bytes"`
		DockerHealthy      *bool    `json:"docker_healthy"`
		BufferedSamples    *int     `json:"buffered_samples"`
		OldestBufferAgeSec *int     `json:"oldest_buffer_age_sec"`
	}
	_ = httpx.Decode(r, &body)

	_, err := s.pool.Exec(r.Context(), `
		UPDATE agents
		SET last_heartbeat_at=now(),
		    status='online',
		    agent_version=COALESCE(NULLIF($2,''), agent_version),
		    last_latency_ms=COALESCE($3, last_latency_ms),
		    resource_cpu_pct=COALESCE($4, resource_cpu_pct),
		    resource_rss_bytes=COALESCE($5, resource_rss_bytes),
		    buffered_samples=COALESCE($6, buffered_samples),
		    oldest_buffer_age_sec=COALESCE($7, oldest_buffer_age_sec),
		    updated_at=now()
		WHERE id=$1`,
		ident.AgentID, body.AgentVersion, body.LatencyMS, body.ResourceCPUPct, body.ResourceRSSBytes,
		body.BufferedSamples, body.OldestBufferAgeSec,
	)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not record heartbeat.")
		return
	}
	_, _ = s.pool.Exec(r.Context(), `
		UPDATE servers
		SET status='online', last_seen_at=now(),
		    docker_available=COALESCE($2, docker_available),
		    updated_at=now()
		WHERE id=$1`, ident.ServerID, body.DockerHealthy)

	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true, "server_time": time.Now().UTC()})
}

func (s *Server) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT id, severity, status, server_id, message, first_seen_at, last_seen_at
		FROM alert_instances
		ORDER BY last_seen_at DESC
		LIMIT 100`)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list alerts.")
		return
	}
	defer rows.Close()
	type alertRow struct {
		ID          uuid.UUID  `json:"id"`
		Severity    string     `json:"severity"`
		Status      string     `json:"status"`
		ServerID    *uuid.UUID `json:"server_id"`
		Message     string     `json:"message"`
		FirstSeenAt time.Time  `json:"first_seen_at"`
		LastSeenAt  time.Time  `json:"last_seen_at"`
	}
	out := make([]alertRow, 0)
	for rows.Next() {
		var row alertRow
		if err := rows.Scan(&row.ID, &row.Severity, &row.Status, &row.ServerID, &row.Message, &row.FirstSeenAt, &row.LastSeenAt); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list alerts.")
			return
		}
		out = append(out, row)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out, "meta": map[string]any{"total": len(out)}})
}

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT id, ts, kind, severity, server_id, message
		FROM infrastructure_events
		ORDER BY ts DESC
		LIMIT 100`)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list events.")
		return
	}
	defer rows.Close()
	type eventRow struct {
		ID       uuid.UUID  `json:"id"`
		TS       time.Time  `json:"ts"`
		Kind     string     `json:"kind"`
		Severity string     `json:"severity"`
		ServerID *uuid.UUID `json:"server_id"`
		Message  string     `json:"message"`
	}
	out := make([]eventRow, 0)
	for rows.Next() {
		var row eventRow
		if err := rows.Scan(&row.ID, &row.TS, &row.Kind, &row.Severity, &row.ServerID, &row.Message); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list events.")
			return
		}
		out = append(out, row)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out, "meta": map[string]any{"total": len(out)}})
}

func (s *Server) handleDockerSummary(w http.ResponseWriter, r *http.Request) {
	var servers, running, stopped, unhealthy, images, volumes, networks int
	_ = s.pool.QueryRow(r.Context(), `
		SELECT COUNT(*) FROM docker_hosts d JOIN servers s ON s.id=d.server_id`).Scan(&servers)
	_ = s.pool.QueryRow(r.Context(), `
		SELECT COUNT(*) FROM containers c JOIN servers s ON s.id=c.server_id WHERE c.state='running'`).Scan(&running)
	_ = s.pool.QueryRow(r.Context(), `
		SELECT COUNT(*) FROM containers c JOIN servers s ON s.id=c.server_id WHERE c.state='exited'`).Scan(&stopped)
	_ = s.pool.QueryRow(r.Context(), `
		SELECT COUNT(*) FROM containers c JOIN servers s ON s.id=c.server_id WHERE c.health='unhealthy'`).Scan(&unhealthy)
	_ = s.pool.QueryRow(r.Context(), `
		SELECT COUNT(*) FROM images i JOIN servers s ON s.id=i.server_id`).Scan(&images)
	_ = s.pool.QueryRow(r.Context(), `
		SELECT COUNT(*) FROM volumes v JOIN servers s ON s.id=v.server_id`).Scan(&volumes)
	_ = s.pool.QueryRow(r.Context(), `
		SELECT COUNT(*) FROM networks n JOIN servers s ON s.id=n.server_id`).Scan(&networks)
	httpx.JSON(w, http.StatusOK, map[string]any{
		"servers": servers, "running": running, "stopped": stopped, "unhealthy": unhealthy,
		"images": images, "volumes": volumes, "networks": networks,
	})
}

func (s *Server) handleListContainers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT c.id, c.server_id, s.name, c.container_id, c.name, c.image_ref, c.state, c.health,
		       c.restart_count, c.compose_project, c.last_seen_at
		FROM containers c
		JOIN servers s ON s.id = c.server_id
		ORDER BY c.name
		LIMIT 500`)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list containers.")
		return
	}
	defer rows.Close()
	type cRow struct {
		ID             uuid.UUID `json:"id"`
		ServerID       uuid.UUID `json:"server_id"`
		ServerName     string    `json:"server_name"`
		ContainerID    string    `json:"container_id"`
		Name           string    `json:"name"`
		ImageRef       string    `json:"image_ref"`
		State          string    `json:"state"`
		Health         string    `json:"health"`
		RestartCount   int       `json:"restart_count"`
		ComposeProject string    `json:"compose_project"`
		LastSeenAt     time.Time `json:"last_seen_at"`
	}
	out := make([]cRow, 0)
	for rows.Next() {
		var row cRow
		if err := rows.Scan(&row.ID, &row.ServerID, &row.ServerName, &row.ContainerID, &row.Name, &row.ImageRef,
			&row.State, &row.Health, &row.RestartCount, &row.ComposeProject, &row.LastSeenAt); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list containers.")
			return
		}
		out = append(out, row)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out, "meta": map[string]any{"total": len(out)}})
}
