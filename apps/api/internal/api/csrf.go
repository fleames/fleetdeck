package api

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/fleetdeck/fleetdeck/apps/api/internal/auth"
	"github.com/fleetdeck/fleetdeck/apps/api/internal/httpx"
)

const csrfCookie = "fleetdeck_csrf"
const csrfHeader = "X-CSRF-Token"

func (s *Server) setCSRFCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true, // compared server-side against header supplied by JS from auth payload
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.CookieSecure,
		MaxAge:   int((7 * 24 * 3600)),
	})
}

func (s *Server) clearCSRFCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.CookieSecure,
		MaxAge:   -1,
	})
}

func (s *Server) issueCSRF(w http.ResponseWriter) (string, error) {
	token, err := auth.NewToken(24)
	if err != nil {
		return "", err
	}
	s.setCSRFCookie(w, token)
	return token, nil
}

func (s *Server) handleCSRF(w http.ResponseWriter, r *http.Request) {
	// Prefer rotating only when missing; return existing cookie value if present.
	if c, err := r.Cookie(csrfCookie); err == nil && c.Value != "" {
		httpx.JSON(w, http.StatusOK, map[string]any{"csrf_token": c.Value})
		return
	}
	token, err := s.issueCSRF(w)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not issue CSRF token.")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"csrf_token": token})
}

func csrfExempt(path string) bool {
	switch path {
	case "/api/v1/auth/login", "/api/v1/auth/bootstrap", "/api/v1/auth/status", "/api/v1/auth/csrf",
		"/install.sh", "/healthz":
		return true
	default:
		return strings.HasPrefix(path, "/agent/v1/") || strings.HasPrefix(path, "/install/")
	}
}

func (s *Server) requireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
			next.ServeHTTP(w, r)
			return
		}
		if csrfExempt(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		cookie, err := r.Cookie(csrfCookie)
		header := r.Header.Get(csrfHeader)
		if err != nil || cookie.Value == "" || header == "" ||
			subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(header)) != 1 {
			httpx.Error(w, http.StatusForbidden, "csrf_failed",
				"Missing or invalid CSRF token. Refresh the page and try again.")
			return
		}
		next.ServeHTTP(w, r)
	})
}
