package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/fleetdeck/fleetdeck/apps/api/internal/auth"
	"github.com/fleetdeck/fleetdeck/apps/api/internal/httpx"
)

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	needs, err := s.auth.NeedsBootstrap(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not read auth status.")
		return
	}
	user, err := s.auth.UserFromRequest(r.Context(), r)
	if err != nil && !errors.Is(err, auth.ErrUnauthenticated) {
		// Transient DB/lookup failures must not look like a logout to the web shell.
		httpx.Error(w, http.StatusServiceUnavailable, "auth_unavailable",
			"Could not verify session. Retry shortly.")
		return
	}
	authenticated := err == nil
	csrf := ""
	if c, err := r.Cookie(csrfCookie); err == nil && c.Value != "" {
		csrf = c.Value
	} else {
		csrf, _ = s.issueCSRF(w)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"bootstrap_required": needs,
		"authenticated":      authenticated,
		"user":               ternaryUser(authenticated, user),
		"csrf_token":         csrf,
	})
}

func ternaryUser(ok bool, u auth.User) any {
	if !ok {
		return nil
	}
	return u
}

func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "Invalid request body.")
		return
	}
	body.Email = strings.TrimSpace(strings.ToLower(body.Email))
	if body.Email == "" || body.Password == "" {
		httpx.Error(w, http.StatusBadRequest, "validation", "Email and password are required.")
		return
	}
	if body.DisplayName == "" {
		body.DisplayName = "Admin"
	}
	u, err := s.auth.Bootstrap(r.Context(), body.Email, body.Password, body.DisplayName)
	if err != nil {
		httpx.Error(w, http.StatusConflict, "bootstrap_failed", err.Error())
		return
	}
	token, err := auth.NewToken(32)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create session.")
		return
	}
	expires := time.Now().UTC().Add(7 * 24 * time.Hour)
	_, err = s.pool.Exec(r.Context(), `
		INSERT INTO sessions (user_id, token_hash, expires_at, ip, user_agent)
		VALUES ($1, $2, $3, $4, $5)`,
		u.ID, auth.HashToken(token), expires, r.RemoteAddr, r.UserAgent(),
	)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create session.")
		return
	}
	s.auth.SetSessionCookie(w, token)
	csrf, _ := s.issueCSRF(w)
	uid := u.ID
	s.auth.Audit(r.Context(), &uid, "auth.bootstrap", "user", u.ID.String(), "ok", r.RemoteAddr, nil)
	httpx.JSON(w, http.StatusCreated, map[string]any{"user": u, "csrf_token": csrf})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !loginLimiter.allow(clientIP(r)) {
		httpx.Error(w, http.StatusTooManyRequests, "rate_limited", "Too many login attempts. Try again shortly.")
		return
	}
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "Invalid request body.")
		return
	}
	u, token, err := s.auth.Login(r.Context(), strings.TrimSpace(strings.ToLower(body.Email)), body.Password, r.RemoteAddr, r.UserAgent())
	if err != nil {
		s.auth.Audit(r.Context(), nil, "auth.login", "user", body.Email, "failed", r.RemoteAddr, nil)
		httpx.Error(w, http.StatusUnauthorized, "invalid_credentials", "Email or password is incorrect.")
		return
	}
	s.auth.SetSessionCookie(w, token)
	csrf, _ := s.issueCSRF(w)
	uid := u.ID
	s.auth.Audit(r.Context(), &uid, "auth.login", "user", u.ID.String(), "ok", r.RemoteAddr, nil)
	httpx.JSON(w, http.StatusOK, map[string]any{"user": u, "csrf_token": csrf})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.SessionCookie); err == nil {
		_ = s.auth.Logout(r.Context(), c.Value)
	}
	s.auth.ClearSessionCookie(w)
	s.clearCSRFCookie(w)
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(ctxUser).(auth.User)
	httpx.JSON(w, http.StatusOK, map[string]any{"user": u})
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var servers, online, offline, warning, criticalAlerts, runningContainers, unhealthyContainers int

	_ = s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM servers`).Scan(&servers)
	_ = s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM servers WHERE status='online'`).Scan(&online)
	_ = s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM servers WHERE status='offline'`).Scan(&offline)
	_ = s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM servers WHERE health_state='warning'`).Scan(&warning)
	_ = s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM alert_instances WHERE status IN ('active','acknowledged') AND severity='critical'`).Scan(&criticalAlerts)
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM containers c JOIN servers s ON s.id=c.server_id WHERE c.state='running'`).Scan(&runningContainers)
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM containers c JOIN servers s ON s.id=c.server_id WHERE c.health='unhealthy'`).Scan(&unhealthyContainers)

	score, factors := healthScore(servers, online, offline, warning, criticalAlerts, unhealthyContainers)

	httpx.JSON(w, http.StatusOK, map[string]any{
		"health": map[string]any{
			"score":   score,
			"max":     100,
			"factors": factors,
		},
		"counts": map[string]any{
			"servers":              servers,
			"online":               online,
			"offline":              offline,
			"warnings":             warning,
			"critical_alerts":      criticalAlerts,
			"running_containers":   runningContainers,
			"unhealthy_containers": unhealthyContainers,
		},
		"generated_at": time.Now().UTC(),
	})
}

type healthFactor struct {
	Code   string `json:"code"`
	Impact int    `json:"impact"`
	Detail string `json:"detail"`
}

func healthScore(servers, online, offline, warning, criticalAlerts, unhealthyContainers int) (int, []healthFactor) {
	if servers == 0 {
		return 100, []healthFactor{{Code: "no_servers", Impact: 0, Detail: "No servers enrolled yet."}}
	}
	score := 100
	factors := []healthFactor{}
	if offline > 0 {
		impact := min(40, offline*20)
		score -= impact
		factors = append(factors, healthFactor{Code: "offline_servers", Impact: -impact, Detail: "One or more servers are offline."})
	}
	if criticalAlerts > 0 {
		impact := min(30, criticalAlerts*15)
		score -= impact
		factors = append(factors, healthFactor{Code: "critical_alerts", Impact: -impact, Detail: "Critical alerts are active."})
	}
	if unhealthyContainers > 0 {
		impact := min(20, unhealthyContainers*5)
		score -= impact
		factors = append(factors, healthFactor{Code: "unhealthy_containers", Impact: -impact, Detail: "Unhealthy containers detected."})
	}
	if warning > 0 {
		impact := min(15, warning*5)
		score -= impact
		factors = append(factors, healthFactor{Code: "warning_servers", Impact: -impact, Detail: "Servers in warning state."})
	}
	if score < 0 {
		score = 0
	}
	_ = online
	return score, factors
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

