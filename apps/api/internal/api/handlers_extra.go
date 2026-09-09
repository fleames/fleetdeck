package api

import (
	"net/http"
	"time"

	"github.com/fleetdeck/fleetdeck/apps/api/internal/auth"
	"github.com/fleetdeck/fleetdeck/apps/api/internal/httpx"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) handleListImages(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT i.id, i.server_id, s.name, i.image_id, i.repository, i.tag, i.size_bytes, i.dangling, i.updated_at
		FROM images i JOIN servers s ON s.id=i.server_id
		ORDER BY i.size_bytes DESC NULLS LAST
		LIMIT 500`)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list images.")
		return
	}
	defer rows.Close()
	type row struct {
		ID         uuid.UUID `json:"id"`
		ServerID   uuid.UUID `json:"server_id"`
		ServerName string    `json:"server_name"`
		ImageID    string    `json:"image_id"`
		Repository string    `json:"repository"`
		Tag        string    `json:"tag"`
		SizeBytes  int64     `json:"size_bytes"`
		Dangling   bool      `json:"dangling"`
		UpdatedAt  time.Time `json:"updated_at"`
	}
	out := make([]row, 0)
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.ID, &x.ServerID, &x.ServerName, &x.ImageID, &x.Repository, &x.Tag, &x.SizeBytes, &x.Dangling, &x.UpdatedAt); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list images.")
			return
		}
		out = append(out, x)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out, "meta": map[string]any{"total": len(out)}})
}

func (s *Server) handleListVolumes(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT v.id, v.server_id, s.name, v.name, v.driver, v.mountpoint, v.unused, v.updated_at
		FROM volumes v JOIN servers s ON s.id=v.server_id
		ORDER BY v.name
		LIMIT 500`)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list volumes.")
		return
	}
	defer rows.Close()
	type row struct {
		ID         uuid.UUID `json:"id"`
		ServerID   uuid.UUID `json:"server_id"`
		ServerName string    `json:"server_name"`
		Name       string    `json:"name"`
		Driver     string    `json:"driver"`
		Mountpoint string    `json:"mountpoint"`
		Unused     bool      `json:"unused"`
		UpdatedAt  time.Time `json:"updated_at"`
	}
	out := make([]row, 0)
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.ID, &x.ServerID, &x.ServerName, &x.Name, &x.Driver, &x.Mountpoint, &x.Unused, &x.UpdatedAt); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list volumes.")
			return
		}
		out = append(out, x)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out, "meta": map[string]any{"total": len(out)}})
}

func (s *Server) handleListNetworks(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT n.id, n.server_id, s.name, n.network_id, n.name, n.driver, n.scope, n.updated_at
		FROM networks n JOIN servers s ON s.id=n.server_id
		ORDER BY n.name
		LIMIT 500`)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list networks.")
		return
	}
	defer rows.Close()
	type row struct {
		ID         uuid.UUID `json:"id"`
		ServerID   uuid.UUID `json:"server_id"`
		ServerName string    `json:"server_name"`
		NetworkID  string    `json:"network_id"`
		Name       string    `json:"name"`
		Driver     string    `json:"driver"`
		Scope      string    `json:"scope"`
		UpdatedAt  time.Time `json:"updated_at"`
	}
	out := make([]row, 0)
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.ID, &x.ServerID, &x.ServerName, &x.NetworkID, &x.Name, &x.Driver, &x.Scope, &x.UpdatedAt); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list networks.")
			return
		}
		out = append(out, x)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out, "meta": map[string]any{"total": len(out)}})
}

func (s *Server) handleListCompose(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT p.id, p.server_id, s.name, p.project_name, p.status, p.updated_at,
			(SELECT COUNT(*) FROM containers c WHERE c.server_id=p.server_id AND c.compose_project=p.project_name) AS containers
		FROM compose_projects p
		JOIN servers s ON s.id=p.server_id
		ORDER BY p.project_name
		LIMIT 500`)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list compose projects.")
		return
	}
	defer rows.Close()
	type row struct {
		ID           uuid.UUID `json:"id"`
		ServerID     uuid.UUID `json:"server_id"`
		ServerName   string    `json:"server_name"`
		ProjectName  string    `json:"project_name"`
		Status       string    `json:"status"`
		UpdatedAt    time.Time `json:"updated_at"`
		Containers   int       `json:"containers"`
	}
	out := make([]row, 0)
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.ID, &x.ServerID, &x.ServerName, &x.ProjectName, &x.Status, &x.UpdatedAt, &x.Containers); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list compose projects.")
			return
		}
		out = append(out, x)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out, "meta": map[string]any{"total": len(out)}})
}

func (s *Server) handleListAlertRules(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT id, name, enabled, severity, metric, operator, threshold, duration_seconds, cooldown_seconds
		FROM alert_rules ORDER BY name`)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list alert rules.")
		return
	}
	defer rows.Close()
	type row struct {
		ID               uuid.UUID `json:"id"`
		Name             string    `json:"name"`
		Enabled          bool      `json:"enabled"`
		Severity         string    `json:"severity"`
		Metric           string    `json:"metric"`
		Operator         string    `json:"operator"`
		Threshold        float64   `json:"threshold"`
		DurationSeconds  int       `json:"duration_seconds"`
		CooldownSeconds  int       `json:"cooldown_seconds"`
	}
	out := make([]row, 0)
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.ID, &x.Name, &x.Enabled, &x.Severity, &x.Metric, &x.Operator, &x.Threshold, &x.DurationSeconds, &x.CooldownSeconds); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list alert rules.")
			return
		}
		out = append(out, x)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

func (s *Server) handleAckAlert(w http.ResponseWriter, r *http.Request) {
	s.mutateAlert(w, r, "acknowledged")
}

func (s *Server) handleResolveAlert(w http.ResponseWriter, r *http.Request) {
	s.mutateAlert(w, r, "resolved")
}

func (s *Server) handleSilenceAlert(w http.ResponseWriter, r *http.Request) {
	s.mutateAlert(w, r, "silenced")
}

func (s *Server) mutateAlert(w http.ResponseWriter, r *http.Request, status string) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation", "Invalid alert id.")
		return
	}
	u := r.Context().Value(ctxUser).(auth.User)
	q := `UPDATE alert_instances SET status=$2, last_seen_at=now()`
	if status == "resolved" {
		q += `, resolved_at=now()`
	}
	q += ` WHERE id=$1`
	tag, err := s.pool.Exec(r.Context(), q, id, status)
	if err != nil || tag.RowsAffected() == 0 {
		httpx.Error(w, http.StatusNotFound, "not_found", "Alert not found.")
		return
	}
	uid := u.ID
	s.auth.Audit(r.Context(), &uid, "alert."+status, "alert", id.String(), "ok", r.RemoteAddr, nil)
	s.hub.Broadcast("alerts.updated", map[string]any{"id": id, "status": status})
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true, "status": status})
}
