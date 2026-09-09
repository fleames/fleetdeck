package api

import (
	"encoding/json"
	"net/http"

	"github.com/fleetdeck/fleetdeck/apps/api/internal/auth"
	"github.com/fleetdeck/fleetdeck/apps/api/internal/httpx"
	"github.com/google/uuid"
)

var defaultDashboardWidgets = []map[string]any{
	{"id": "health", "visible": true},
	{"id": "counts", "visible": true},
	{"id": "servers", "visible": true},
}

func (s *Server) handleGetDashboardLayout(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(ctxUser).(auth.User)
	var id uuid.UUID
	var name string
	var layout json.RawMessage
	var isDefault bool
	err := s.pool.QueryRow(r.Context(), `
		SELECT id, name, layout, is_default FROM dashboard_layouts
		WHERE user_id=$1
		ORDER BY is_default DESC, name ASC
		LIMIT 1`, u.ID,
	).Scan(&id, &name, &layout, &isDefault)
	if err != nil {
		httpx.JSON(w, http.StatusOK, map[string]any{
			"name":       "Default",
			"is_default": true,
			"layout":     defaultDashboardWidgets,
			"persisted":  false,
		})
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"id": id, "name": name, "is_default": isDefault, "layout": layout, "persisted": true,
	})
}

func (s *Server) handlePutDashboardLayout(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(ctxUser).(auth.User)
	var body struct {
		Name   string          `json:"name"`
		Layout json.RawMessage `json:"layout"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "Invalid layout body.")
		return
	}
	if len(body.Layout) == 0 {
		httpx.Error(w, http.StatusBadRequest, "validation", "layout is required.")
		return
	}
	var parsed []map[string]any
	if err := json.Unmarshal(body.Layout, &parsed); err != nil || len(parsed) == 0 {
		httpx.Error(w, http.StatusBadRequest, "validation", "layout must be a non-empty JSON array.")
		return
	}
	allowed := map[string]struct{}{"health": {}, "counts": {}, "servers": {}}
	cleaned := make([]map[string]any, 0, len(parsed))
	seen := map[string]struct{}{}
	for _, item := range parsed {
		id, _ := item["id"].(string)
		if _, ok := allowed[id]; !ok {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		visible := true
		if v, ok := item["visible"].(bool); ok {
			visible = v
		}
		cleaned = append(cleaned, map[string]any{"id": id, "visible": visible})
	}
	for _, id := range []string{"health", "counts", "servers"} {
		if _, ok := seen[id]; !ok {
			cleaned = append(cleaned, map[string]any{"id": id, "visible": true})
		}
	}
	raw, _ := json.Marshal(cleaned)
	name := body.Name
	if name == "" {
		name = "Default"
	}
	var id uuid.UUID
	err := s.pool.QueryRow(r.Context(), `
		INSERT INTO dashboard_layouts (user_id, name, layout, is_default)
		VALUES ($1,$2,$3::jsonb,true)
		ON CONFLICT (user_id, name) DO UPDATE SET layout=EXCLUDED.layout, is_default=true
		RETURNING id`, u.ID, name, string(raw),
	).Scan(&id)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not save layout.")
		return
	}
	uid := u.ID
	s.auth.Audit(r.Context(), &uid, "dashboard.layout.save", "dashboard_layout", id.String(), "ok", r.RemoteAddr, nil)
	httpx.JSON(w, http.StatusOK, map[string]any{
		"id": id, "name": name, "layout": cleaned, "persisted": true,
	})
}
