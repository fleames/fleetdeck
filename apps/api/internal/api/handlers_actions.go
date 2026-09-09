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
)

var allowedContainerActions = map[string]string{
	"start":   "container.start",
	"stop":    "container.stop",
	"restart": "container.restart",
	"pause":   "container.pause",
	"unpause": "container.unpause",
}

func (s *Server) handleContainerAction(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(ctxUser).(auth.User)
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation", "Invalid container id.")
		return
	}
	action := strings.ToLower(chi.URLParam(r, "action"))
	cmdType, ok := allowedContainerActions[action]
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "validation", "Unsupported action. Use start, stop, restart, pause, or unpause.")
		return
	}
	var body struct {
		Confirm bool `json:"confirm"`
	}
	if err := httpx.Decode(r, &body); err != nil || !body.Confirm {
		httpx.Error(w, http.StatusBadRequest, "confirmation_required",
			"Management actions require confirm=true in the request body.")
		return
	}

	var serverID, agentID uuid.UUID
	var dockerID, name, serverName string
	err = s.pool.QueryRow(r.Context(), `
		SELECT c.server_id, a.id, c.container_id, c.name, s.name
		FROM containers c
		JOIN agents a ON a.server_id=c.server_id
		JOIN servers s ON s.id=c.server_id
		WHERE c.id=$1`, id,
	).Scan(&serverID, &agentID, &dockerID, &name, &serverName)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "Container or agent not found.")
		return
	}

	payload, _ := json.Marshal(map[string]any{"container_id": dockerID, "action": action})
	var cmdID uuid.UUID
	err = s.pool.QueryRow(r.Context(), `
		INSERT INTO agent_commands (agent_id, server_id, type, payload, expires_at)
		VALUES ($1,$2,$3,$4::jsonb, now() + interval '90 seconds')
		RETURNING id`, agentID, serverID, cmdType, string(payload),
	).Scan(&cmdID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not enqueue action.")
		return
	}

	uid := u.ID
	s.auth.Audit(r.Context(), &uid, "container."+action, "container", id.String(), "queued", r.RemoteAddr, map[string]any{
		"name": name, "server": serverName, "command_id": cmdID,
	})
	_, _ = s.pool.Exec(r.Context(), `
		INSERT INTO infrastructure_events (kind, severity, server_id, container_id, message, context)
		VALUES ($1,'info',$2,$3,$4,$5::jsonb)`,
		"container."+action, serverID, id,
		"Queued "+action+" for container "+name,
		string(mustJSON(map[string]any{"command_id": cmdID, "user": u.Email})),
	)

	status, result, errText, timedOut := s.waitCommand(r, cmdID, 30*time.Second)
	if timedOut {
		httpx.JSON(w, http.StatusAccepted, map[string]any{
			"accepted":   true,
			"command_id": cmdID,
			"status":     "pending",
			"message":    "Action queued; agent has not completed yet.",
		})
		return
	}
	if status == "failed" || status == "expired" {
		msg := "Container action failed."
		if errText != "" {
			msg = errText
		}
		s.auth.Audit(r.Context(), &uid, "container."+action, "container", id.String(), "failed", r.RemoteAddr, map[string]any{
			"error": msg, "command_id": cmdID,
		})
		httpx.ErrorDetails(w, http.StatusBadGateway, "action_failed", msg,
			map[string]any{"command_id": cmdID}, nil)
		return
	}
	s.auth.Audit(r.Context(), &uid, "container."+action, "container", id.String(), "ok", r.RemoteAddr, map[string]any{
		"command_id": cmdID,
	})
	httpx.JSON(w, http.StatusOK, map[string]any{
		"ok": true, "action": action, "command_id": cmdID, "result": result, "status": status,
	})
}

func (s *Server) handleContainerEnv(w http.ResponseWriter, r *http.Request) {
	u, _ := r.Context().Value(ctxUser).(auth.User)
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation", "Invalid container id.")
		return
	}
	reveal := r.URL.Query().Get("reveal") == "1" || r.URL.Query().Get("reveal_env") == "1"
	if reveal && u.Role != "admin" {
		httpx.Error(w, http.StatusForbidden, "forbidden", "Revealing environment variables requires an admin role.")
		return
	}

	var serverID, agentID uuid.UUID
	var dockerID, name string
	err = s.pool.QueryRow(r.Context(), `
		SELECT c.server_id, a.id, c.container_id, c.name
		FROM containers c JOIN agents a ON a.server_id=c.server_id WHERE c.id=$1`, id,
	).Scan(&serverID, &agentID, &dockerID, &name)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "Container or agent not found.")
		return
	}

	payload, _ := json.Marshal(map[string]any{"container_id": dockerID})
	var cmdID uuid.UUID
	err = s.pool.QueryRow(r.Context(), `
		INSERT INTO agent_commands (agent_id, server_id, type, payload, expires_at)
		VALUES ($1,$2,'container.inspect',$3::jsonb, now() + interval '90 seconds')
		RETURNING id`, agentID, serverID, string(payload),
	).Scan(&cmdID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not request inspect.")
		return
	}

	status, result, errText, timedOut := s.waitCommand(r, cmdID, 25*time.Second)
	if timedOut {
		httpx.ErrorDetails(w, http.StatusGatewayTimeout, "agent_timeout",
			"The monitoring agent did not return container inspect in time.",
			map[string]any{"command_id": cmdID}, nil)
		return
	}
	if status != "completed" {
		msg := "Inspect failed."
		if errText != "" {
			msg = errText
		}
		httpx.Error(w, http.StatusBadGateway, "inspect_failed", msg)
		return
	}

	var inspect struct {
		Env []string `json:"Env"`
	}
	_ = json.Unmarshal([]byte(result), &inspect)
	entries := make([]map[string]any, 0, len(inspect.Env))
	for _, line := range inspect.Env {
		key, val, _ := strings.Cut(line, "=")
		sensitive := isSensitiveEnvKey(key)
		item := map[string]any{"key": key, "sensitive": sensitive}
		if sensitive && !reveal {
			item["value"] = "••••••••"
			item["masked"] = true
		} else {
			item["value"] = val
			item["masked"] = false
		}
		entries = append(entries, item)
	}

	if reveal {
		uid := u.ID
		s.auth.Audit(r.Context(), &uid, "container.env.reveal", "container", id.String(), "ok", r.RemoteAddr, map[string]any{
			"name": name, "count": len(entries),
		})
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"container_id": id,
		"name":         name,
		"revealed":     reveal,
		"env":          entries,
		"note":         "Sensitive keys are masked by default. Reveal requires admin and is audited.",
	})
}

func (s *Server) waitCommand(r *http.Request, cmdID uuid.UUID, timeout time.Duration) (status, result, errText string, timedOut bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var res, errPtr *string
		err := s.pool.QueryRow(r.Context(), `
			SELECT status, result_text, NULLIF(error,'') FROM agent_commands WHERE id=$1`, cmdID,
		).Scan(&status, &res, &errPtr)
		if err != nil {
			return "failed", "", err.Error(), false
		}
		if res != nil {
			result = *res
		}
		if errPtr != nil {
			errText = *errPtr
		}
		if status == "completed" || status == "failed" || status == "expired" {
			return status, result, errText, false
		}
		select {
		case <-r.Context().Done():
			return status, result, errText, true
		case <-time.After(400 * time.Millisecond):
		}
	}
	return status, result, errText, true
}

func isSensitiveEnvKey(key string) bool {
	u := strings.ToUpper(key)
	needles := []string{"PASSWORD", "SECRET", "TOKEN", "API_KEY", "PRIVATE", "CREDENTIAL", "AUTH", "PASSWD", "ACCESS_KEY"}
	for _, n := range needles {
		if strings.Contains(u, n) {
			return true
		}
	}
	return false
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
