package api

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/fleetdeck/fleetdeck/apps/api/internal/auth"
	"github.com/fleetdeck/fleetdeck/apps/api/internal/httpx"
	"github.com/go-chi/chi/v5"
)

// Allowlisted secret kinds — keep the envelope store from becoming an arbitrary blob dump.
var allowedSecretKinds = map[string]bool{
	"webhook": true,
}

var secretNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func validSecretKindName(kind, name string) bool {
	return allowedSecretKinds[kind] && secretNameRe.MatchString(name)
}

func (s *Server) handleListSecrets(w http.ResponseWriter, r *http.Request) {
	if s.secrets == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "secrets_unavailable", "Secrets envelope is not configured (check SESSION_SECRET).")
		return
	}
	meta, err := s.secrets.List(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list secrets.")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"data": meta,
		"note": "Plaintext is never returned. PUT/DELETE rotate named secrets (kind webhook only).",
	})
}

func (s *Server) handlePutSecret(w http.ResponseWriter, r *http.Request) {
	if s.secrets == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "secrets_unavailable", "Secrets envelope is not configured (check SESSION_SECRET).")
		return
	}
	kind := strings.TrimSpace(chi.URLParam(r, "kind"))
	name := strings.TrimSpace(chi.URLParam(r, "name"))
	if !validSecretKindName(kind, name) {
		httpx.Error(w, http.StatusBadRequest, "validation", "Invalid secret kind or name. Allowed kind: webhook; name: snake_case.")
		return
	}
	var body struct {
		Value string `json:"value"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "Invalid request body.")
		return
	}
	if strings.TrimSpace(body.Value) == "" {
		httpx.Error(w, http.StatusBadRequest, "validation", "value is required.")
		return
	}
	id, err := s.secrets.Put(r.Context(), kind, name, []byte(body.Value))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not store secret.")
		return
	}
	u := r.Context().Value(ctxUser).(auth.User)
	uid := u.ID
	s.auth.Audit(r.Context(), &uid, "secret.put", "secret", kind+"/"+name, "ok", r.RemoteAddr, nil)
	httpx.JSON(w, http.StatusOK, map[string]any{
		"id":   id,
		"kind": kind,
		"name": name,
		"ok":   true,
	})
}

func (s *Server) handleDeleteSecret(w http.ResponseWriter, r *http.Request) {
	if s.secrets == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "secrets_unavailable", "Secrets envelope is not configured (check SESSION_SECRET).")
		return
	}
	kind := strings.TrimSpace(chi.URLParam(r, "kind"))
	name := strings.TrimSpace(chi.URLParam(r, "name"))
	if !validSecretKindName(kind, name) {
		httpx.Error(w, http.StatusBadRequest, "validation", "Invalid secret kind or name.")
		return
	}
	if err := s.secrets.Delete(r.Context(), kind, name); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not delete secret.")
		return
	}
	u := r.Context().Value(ctxUser).(auth.User)
	uid := u.ID
	s.auth.Audit(r.Context(), &uid, "secret.delete", "secret", kind+"/"+name, "ok", r.RemoteAddr, nil)
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true, "kind": kind, "name": name})
}

func (s *Server) handleSecretStatus(w http.ResponseWriter, r *http.Request) {
	if s.secrets == nil {
		httpx.JSON(w, http.StatusOK, map[string]any{
			"configured":    false,
			"alert_signing": false,
		})
		return
	}
	exists, err := s.secrets.Exists(r.Context(), "webhook", "alert_signing")
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not read secret status.")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"configured":    true,
		"alert_signing": exists,
	})
}
