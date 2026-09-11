package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fleetdeck/fleetdeck/apps/api/internal/auth"
	"github.com/fleetdeck/fleetdeck/apps/api/internal/httpx"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) handleGetContainer(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation", "Invalid container id.")
		return
	}
	var out struct {
		ID             uuid.UUID       `json:"id"`
		ServerID       uuid.UUID       `json:"server_id"`
		ServerName     string          `json:"server_name"`
		ContainerID    string          `json:"container_id"`
		Name           string          `json:"name"`
		ImageRef       string          `json:"image_ref"`
		ImageID        string          `json:"image_id"`
		State          string          `json:"state"`
		Health         string          `json:"health"`
		RestartCount   int             `json:"restart_count"`
		ComposeProject string          `json:"compose_project"`
		ComposeService string          `json:"compose_service"`
		Labels         json.RawMessage `json:"labels"`
		Ports          json.RawMessage `json:"ports"`
		StartedAt      *time.Time      `json:"started_at"`
		CreatedAt      *time.Time      `json:"container_created_at"`
		LastSeenAt     time.Time       `json:"last_seen_at"`
		CPUPct         *float64        `json:"cpu_pct"`
		MemUsedBytes   *int64          `json:"mem_used_bytes"`
		MemLimitBytes  *int64          `json:"mem_limit_bytes"`
		MetricsAt      *time.Time      `json:"metrics_at"`
	}
	err = s.pool.QueryRow(r.Context(), `
		SELECT c.id, c.server_id, s.name, c.container_id, c.name, c.image_ref, c.image_id, c.state, c.health,
		       c.restart_count, c.compose_project, c.compose_service, c.labels, c.ports,
		       c.started_at, c.container_created_at, c.last_seen_at,
		       m.cpu_pct, m.mem_used_bytes, m.mem_limit_bytes, m.ts
		FROM containers c
		JOIN servers s ON s.id=c.server_id
		LEFT JOIN LATERAL (
			SELECT * FROM container_metrics_raw cm WHERE cm.container_id=c.id ORDER BY cm.ts DESC LIMIT 1
		) m ON true
		WHERE c.id=$1`, id,
	).Scan(&out.ID, &out.ServerID, &out.ServerName, &out.ContainerID, &out.Name, &out.ImageRef, &out.ImageID,
		&out.State, &out.Health, &out.RestartCount, &out.ComposeProject, &out.ComposeService, &out.Labels, &out.Ports,
		&out.StartedAt, &out.CreatedAt, &out.LastSeenAt, &out.CPUPct, &out.MemUsedBytes, &out.MemLimitBytes, &out.MetricsAt)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "Container not found.")
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (s *Server) handleContainerLogs(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation", "Invalid container id.")
		return
	}
	tail := 200
	if v := r.URL.Query().Get("tail"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 2000 {
			tail = n
		}
	}

	var serverID, agentID uuid.UUID
	var dockerContainerID, name string
	err = s.pool.QueryRow(r.Context(), `
		SELECT c.server_id, a.id, c.container_id, c.name
		FROM containers c
		JOIN agents a ON a.server_id=c.server_id
		WHERE c.id=$1`, id,
	).Scan(&serverID, &agentID, &dockerContainerID, &name)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "Container or agent not found.")
		return
	}

	payload, _ := json.Marshal(map[string]any{
		"container_id": dockerContainerID,
		"tail":         tail,
	})
	var cmdID uuid.UUID
	err = s.pool.QueryRow(r.Context(), `
		INSERT INTO agent_commands (agent_id, server_id, type, payload, expires_at)
		VALUES ($1,$2,'container.logs',$3::jsonb, now() + interval '90 seconds')
		RETURNING id`, agentID, serverID, string(payload),
	).Scan(&cmdID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not request logs.")
		return
	}

	// Wait up to 25s for agent result
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		var status string
		var result, errText *string
		err := s.pool.QueryRow(r.Context(), `
			SELECT status, result_text, NULLIF(error,'') FROM agent_commands WHERE id=$1`, cmdID,
		).Scan(&status, &result, &errText)
		if err != nil {
			break
		}
		if status == "completed" {
			text := ""
			if result != nil {
				text = *result
			}
			httpx.JSON(w, http.StatusOK, map[string]any{
				"container_id": id,
				"name":         name,
				"tail":         tail,
				"lines":        splitLogLines(text),
				"raw":          text,
				"fetched_at":   time.Now().UTC(),
			})
			return
		}
		if status == "failed" || status == "expired" {
			msg := "Log collection failed."
			if errText != nil && *errText != "" {
				msg = *errText
			}
			httpx.Error(w, http.StatusBadGateway, "logs_unavailable", msg)
			return
		}
		select {
		case <-r.Context().Done():
			httpx.Error(w, http.StatusRequestTimeout, "cancelled", "Request cancelled.")
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
	httpx.ErrorDetails(w, http.StatusGatewayTimeout, "agent_timeout",
		"The monitoring agent did not return container logs in time.",
		map[string]any{"command_id": cmdID, "server_id": serverID},
		nil)
}

func splitLogLines(raw string) []string {
	if raw == "" {
		return []string{}
	}
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	parts := strings.Split(raw, "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func (s *Server) handleAgentPollCommands(w http.ResponseWriter, r *http.Request) {
	ident := r.Context().Value(ctxAgent).(agentIdentity)
	_, _ = s.pool.Exec(r.Context(), `
		UPDATE agent_commands SET status='expired'
		WHERE agent_id=$1 AND status='pending' AND expires_at < now()`, ident.AgentID)

	type cmd struct {
		ID        uuid.UUID       `json:"id"`
		Type      string          `json:"type"`
		Payload   json.RawMessage `json:"payload"`
		CreatedAt time.Time       `json:"created_at"`
		ExpiresAt time.Time       `json:"expires_at"`
	}
	wait := agentCommandWait(r)
	deadline := time.Now().Add(wait)
	for {
		rows, err := s.pool.Query(r.Context(), `
			SELECT id, type, payload, created_at, expires_at
			FROM agent_commands
			WHERE agent_id=$1 AND status='pending'
			ORDER BY created_at ASC
			LIMIT 10`, ident.AgentID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list commands.")
			return
		}
		out := make([]cmd, 0)
		ids := make([]uuid.UUID, 0)
		for rows.Next() {
			var c cmd
			if err := rows.Scan(&c.ID, &c.Type, &c.Payload, &c.CreatedAt, &c.ExpiresAt); err != nil {
				rows.Close()
				httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list commands.")
				return
			}
			out = append(out, c)
			ids = append(ids, c.ID)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list commands.")
			return
		}
		if len(out) > 0 || wait == 0 || !time.Now().Before(deadline) {
			for _, id := range ids {
				_, _ = s.pool.Exec(r.Context(), `UPDATE agent_commands SET status='running' WHERE id=$1 AND status='pending'`, id)
			}
			httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
			return
		}
		remaining := time.Until(deadline)
		pause := time.Second
		if remaining < pause {
			pause = remaining
		}
		timer := time.NewTimer(pause)
		select {
		case <-r.Context().Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func agentCommandWait(r *http.Request) time.Duration {
	const maxWait = 30 * time.Second
	raw := r.URL.Query().Get("wait")
	if raw == "" {
		return 0
	}
	wait, err := time.ParseDuration(raw)
	if err != nil || wait <= 0 {
		return 0
	}
	if wait > maxWait {
		return maxWait
	}
	return wait
}

func (s *Server) handleAgentCommandResult(w http.ResponseWriter, r *http.Request) {
	ident := r.Context().Value(ctxAgent).(agentIdentity)
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation", "Invalid command id.")
		return
	}
	var body struct {
		OK     bool   `json:"ok"`
		Result string `json:"result"`
		Error  string `json:"error"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "Invalid command result.")
		return
	}
	status := "completed"
	if !body.OK {
		status = "failed"
	}
	// Cap result size (2MB)
	if len(body.Result) > 2_000_000 {
		body.Result = body.Result[:2_000_000]
	}
	var cmdType string
	var payload []byte
	var serverID uuid.UUID
	err = s.pool.QueryRow(r.Context(), `
		UPDATE agent_commands
		SET status=$3, result_text=$4, error=$5, completed_at=now()
		WHERE id=$1 AND agent_id=$2 AND status IN ('pending','running')
		RETURNING type, payload, server_id`,
		id, ident.AgentID, status, body.Result, body.Error,
	).Scan(&cmdType, &payload, &serverID)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "Command not found.")
		return
	}
	if body.OK && cmdType == "container.remove" {
		var p struct {
			ContainerID string `json:"container_id"`
		}
		_ = json.Unmarshal(payload, &p)
		if p.ContainerID != "" {
			s.deleteContainerInventory(r.Context(), serverID, p.ContainerID)
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

type searchHit struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	Href     string `json:"href"`
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 1 {
		httpx.JSON(w, http.StatusOK, map[string]any{
			"servers": []searchHit{}, "containers": []searchHit{}, "images": []searchHit{},
			"compose": []searchHit{}, "volumes": []searchHit{}, "networks": []searchHit{}, "alerts": []searchHit{}, "events": []searchHit{},
		})
		return
	}
	like := "%" + strings.ToLower(q) + "%"

	servers := queryHits(r.Context(), s, `
		SELECT id::text, name, COALESCE(NULLIF(hostname,''), status)
		FROM servers
		WHERE lower(name) LIKE $1 OR lower(hostname) LIKE $1 OR lower(primary_address) LIKE $1
		ORDER BY name LIMIT 8`, like, func(id, title, sub string) searchHit {
		return searchHit{ID: id, Title: title, Subtitle: sub, Href: "/servers/" + id}
	})
	containers := queryHits(r.Context(), s, `
		SELECT c.id::text, COALESCE(NULLIF(c.name,''), left(c.container_id,12)), s.name || ' · ' || c.image_ref
		FROM containers c JOIN servers s ON s.id=c.server_id
		WHERE lower(c.name) LIKE $1 OR lower(c.image_ref) LIKE $1 OR lower(c.compose_project) LIKE $1
		ORDER BY c.name LIMIT 10`, like, func(id, title, sub string) searchHit {
		return searchHit{ID: id, Title: title, Subtitle: sub, Href: "/containers/" + id}
	})
	images := queryHits(r.Context(), s, `
		SELECT i.id::text, i.repository || ':' || i.tag, s.name
		FROM images i JOIN servers s ON s.id=i.server_id
		WHERE lower(i.repository) LIKE $1 OR lower(i.tag) LIKE $1
		ORDER BY i.repository LIMIT 8`, like, func(id, title, sub string) searchHit {
		return searchHit{ID: id, Title: title, Subtitle: sub, Href: "/images"}
	})
	compose := queryHits(r.Context(), s, `
		SELECT p.id::text, p.project_name, s.name
		FROM compose_projects p JOIN servers s ON s.id=p.server_id
		WHERE lower(p.project_name) LIKE $1
		ORDER BY p.project_name LIMIT 8`, like, func(id, title, sub string) searchHit {
		return searchHit{ID: id, Title: title, Subtitle: sub, Href: "/compose"}
	})
	volumes := queryHits(r.Context(), s, `
		SELECT v.id::text, v.name, s.name
		FROM volumes v JOIN servers s ON s.id=v.server_id
		WHERE lower(v.name) LIKE $1
		ORDER BY v.name LIMIT 8`, like, func(id, title, sub string) searchHit {
		return searchHit{ID: id, Title: title, Subtitle: sub, Href: "/volumes"}
	})
	networks := queryHits(r.Context(), s, `
		SELECT n.id::text, n.name, s.name
		FROM networks n JOIN servers s ON s.id=n.server_id
		WHERE lower(n.name) LIKE $1
		ORDER BY n.name LIMIT 8`, like, func(id, title, sub string) searchHit {
		return searchHit{ID: id, Title: title, Subtitle: sub, Href: "/networks"}
	})
	alerts := queryHits(r.Context(), s, `
		SELECT id::text, message, severity || ' · ' || status
		FROM alert_instances
		WHERE lower(message) LIKE $1
		ORDER BY last_seen_at DESC LIMIT 8`, like, func(id, title, sub string) searchHit {
		return searchHit{ID: id, Title: title, Subtitle: sub, Href: "/alerts"}
	})
	events := queryHits(r.Context(), s, `
		SELECT id::text, message, kind
		FROM infrastructure_events
		WHERE lower(message) LIKE $1 OR lower(kind) LIKE $1
		ORDER BY ts DESC LIMIT 8`, like, func(id, title, sub string) searchHit {
		return searchHit{ID: id, Title: title, Subtitle: sub, Href: "/events"}
	})

	httpx.JSON(w, http.StatusOK, map[string]any{
		"q":       q,
		"servers": servers, "containers": containers, "images": images,
		"compose": compose, "volumes": volumes, "networks": networks,
		"alerts": alerts, "events": events,
	})
}

func queryHits(ctx context.Context, s *Server, sql string, like string, mapHit func(id, title, sub string) searchHit) []searchHit {
	rows, err := s.pool.Query(ctx, sql, like)
	if err != nil {
		return []searchHit{}
	}
	defer rows.Close()
	out := make([]searchHit, 0)
	for rows.Next() {
		var id, title, sub string
		if err := rows.Scan(&id, &title, &sub); err != nil {
			continue
		}
		out = append(out, mapHit(id, title, sub))
	}
	return out
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `SELECT key, value FROM settings ORDER BY key`)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not load settings.")
		return
	}
	defer rows.Close()
	out := map[string]json.RawMessage{}
	for rows.Next() {
		var k string
		var v json.RawMessage
		if err := rows.Scan(&k, &v); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not load settings.")
			return
		}
		out[k] = v
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (s *Server) handlePatchSettings(w http.ResponseWriter, r *http.Request) {
	var body map[string]json.RawMessage
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "Invalid settings payload.")
		return
	}
	allowed := map[string]bool{"general": true, "metrics": true, "alerts": true}
	u := r.Context().Value(ctxUser).(auth.User)
	for k, v := range body {
		if !allowed[k] {
			continue
		}
		_, err := s.pool.Exec(r.Context(), `
			INSERT INTO settings(key, value, updated_at) VALUES ($1,$2::jsonb,now())
			ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()`, k, string(v))
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not save settings.")
			return
		}
	}
	uid := u.ID
	s.auth.Audit(r.Context(), &uid, "settings.update", "settings", "", "ok", r.RemoteAddr, nil)
	s.handleGetSettings(w, r)
}

func (s *Server) handleListAudit(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT a.id, a.ts, a.action, a.target_type, a.target_id, a.result, a.ip, COALESCE(u.email,'')
		FROM audit_logs a
		LEFT JOIN users u ON u.id=a.user_id
		ORDER BY a.ts DESC
		LIMIT 200`)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list audit logs.")
		return
	}
	defer rows.Close()
	type row struct {
		ID         uuid.UUID `json:"id"`
		TS         time.Time `json:"ts"`
		Action     string    `json:"action"`
		TargetType string    `json:"target_type"`
		TargetID   string    `json:"target_id"`
		Result     string    `json:"result"`
		IP         string    `json:"ip"`
		UserEmail  string    `json:"user_email"`
	}
	out := make([]row, 0)
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.ID, &x.TS, &x.Action, &x.TargetType, &x.TargetID, &x.Result, &x.IP, &x.UserEmail); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list audit logs.")
			return
		}
		out = append(out, x)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}
