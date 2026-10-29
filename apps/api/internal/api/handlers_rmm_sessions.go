package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fleetdeck/fleetdeck/apps/api/internal/auth"
	"github.com/fleetdeck/fleetdeck/apps/api/internal/httpx"
	"github.com/fleetdeck/fleetdeck/apps/api/internal/rmm"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

var rmmSessionLimiter *dbRateLimiter

// supersedeOpenRMMSessions closes any non-ended sessions for hostID, notifies the agent,
// and returns how many were closed. Used when the operator clicks Connect again.
func (s *Server) supersedeOpenRMMSessions(ctx context.Context, hostID uuid.UUID, reason string) int {
	rows, err := s.pool.Query(ctx, `
		SELECT id FROM rmm_sessions
		WHERE host_id=$1 AND status IN ('pending','connecting','active')
		ORDER BY created_at ASC`, hostID)
	if err != nil {
		log.Printf("rmm supersede list host %s: %v", hostID, err)
		return 0
	}
	defer rows.Close()

	var closed int
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			continue
		}
		s.rmmHub.Close(id)
		s.clearRMMSessionCredOverride(id)
		tag, err := s.pool.Exec(ctx, `
			UPDATE rmm_sessions
			SET status='closed', ended_at=now(), updated_at=now(),
			    error=COALESCE(NULLIF(error, ''), $2)
			WHERE id=$1 AND status IN ('pending','connecting','active')`, id, reason)
		if err != nil || tag.RowsAffected() == 0 {
			continue
		}
		closed++
		_, _ = s.pool.Exec(ctx, `
			INSERT INTO rmm_commands (host_id, type, payload, expires_at)
			VALUES ($1, 'session.close', jsonb_build_object('session_id', $2::text), now() + interval '2 minutes')`,
			hostID, id.String())
		_, _ = s.pool.Exec(ctx, `
			INSERT INTO rmm_session_events (session_id, kind, message)
			VALUES ($1, 'session.superseded', $2)`, id, reason)
		s.hub.Broadcast("rmm.session.updated", map[string]any{
			"session_id": id, "host_id": hostID, "status": "closed", "reason": "superseded",
		})
	}
	return closed
}

// closeRMMSession marks a session closed, tears down the hub tunnel, and queues session.close.
func (s *Server) closeRMMSession(ctx context.Context, sessionID, hostID uuid.UUID, eventKind, message, errMsg string) {
	s.rmmHub.Close(sessionID)
	s.clearRMMSessionCredOverride(sessionID)
	_, _ = s.pool.Exec(ctx, `
		UPDATE rmm_sessions
		SET status='closed', ended_at=now(), updated_at=now(),
		    error=CASE WHEN $2 = '' THEN error ELSE $2 END
		WHERE id=$1 AND status IN ('pending','connecting','active')`, sessionID, errMsg)
	_, _ = s.pool.Exec(ctx, `
		INSERT INTO rmm_commands (host_id, type, payload, expires_at)
		VALUES ($1, 'session.close', jsonb_build_object('session_id', $2::text), now() + interval '2 minutes')`,
		hostID, sessionID.String())
	if eventKind != "" {
		_, _ = s.pool.Exec(ctx, `
			INSERT INTO rmm_session_events (session_id, kind, message)
			VALUES ($1, $2, $3)`, sessionID, eventKind, message)
	}
	s.hub.Broadcast("rmm.session.updated", map[string]any{
		"session_id": sessionID, "host_id": hostID, "status": "closed",
	})
}

func (s *Server) handleCreateRMMBrowserSession(w http.ResponseWriter, r *http.Request) {
	hostID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation", "Invalid host id.")
		return
	}
	var body struct {
		Confirm bool `json:"confirm"`
	}
	if err := httpx.Decode(r, &body); err != nil || !body.Confirm {
		httpx.Error(w, http.StatusBadRequest, "confirmation_required",
			"Starting an RDP session requires confirm=true.")
		return
	}
	if rmmSessionLimiter != nil && !rmmSessionLimiter.allow(r.Context(), clientIP(r)) {
		httpx.Error(w, http.StatusTooManyRequests, "rate_limited", "Too many session attempts. Try again shortly.")
		return
	}

	u := r.Context().Value(ctxUser).(auth.User)
	var status string
	var rdpEnabled bool
	var concurrent bool
	var productType, osVersion string
	err = s.pool.QueryRow(r.Context(), `
		SELECT status, rdp_enabled, rdp_concurrent_sessions, os_product_type, os_version
		FROM rmm_hosts WHERE id=$1`, hostID).
		Scan(&status, &rdpEnabled, &concurrent, &productType, &osVersion)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "RMM host not found.")
		return
	}
	if status != "online" {
		httpx.Error(w, http.StatusConflict, "host_offline", "Host is offline. Wait for the RMM agent heartbeat.")
		return
	}
	if !rdpEnabled {
		httpx.Error(w, http.StatusConflict, "rdp_disabled",
			"Remote Desktop appears disabled on the host. Enable RDP and ensure the agent can reach 127.0.0.1:3389.")
		return
	}

	if n := s.supersedeOpenRMMSessions(r.Context(), hostID, "superseded by new browser session"); n > 0 {
		log.Printf("rmm: superseded %d open session(s) on host %s before browser create", n, hostID)
		uid := u.ID
		s.auth.Audit(r.Context(), &uid, "rmm.session.supersede", "rmm_host", hostID.String(), "ok", r.RemoteAddr,
			map[string]any{"count": n, "mode": "browser"})
	}

	tunnelToken, err := auth.NewToken(24)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create session.")
		return
	}
	expires := time.Now().UTC().Add(4 * time.Hour)
	cmdExpires := time.Now().UTC().Add(2 * time.Minute)

	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create session.")
		return
	}
	defer tx.Rollback(r.Context())

	var sessionID uuid.UUID
	err = tx.QueryRow(r.Context(), `
		INSERT INTO rmm_sessions (host_id, mode, status, created_by, tunnel_token_hash, expires_at)
		VALUES ($1, 'browser', 'pending', $2, $3, $4)
		RETURNING id`, hostID, u.ID, auth.HashToken(tunnelToken), expires,
	).Scan(&sessionID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create session.")
		return
	}

	tunnelURL := strings.TrimRight(s.cfg.APIPublicURL, "/") + "/rmm/v1/tunnel"
	payload, _ := json.Marshal(map[string]any{
		"session_id":   sessionID,
		"tunnel_url":   tunnelURL,
		"tunnel_token": tunnelToken,
		"rdp_host":     "127.0.0.1",
		"mode":         "browser",
	})
	_, err = tx.Exec(r.Context(), `
		INSERT INTO rmm_commands (host_id, type, payload, expires_at)
		VALUES ($1, 'session.create', $2::jsonb, $3)`, hostID, string(payload), cmdExpires)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not queue session command.")
		return
	}
	_, _ = tx.Exec(r.Context(), `
		INSERT INTO rmm_session_events (session_id, kind, message, context)
		VALUES ($1, 'session.created', 'Browser RDP session requested.', jsonb_build_object('user_id', $2::text))`,
		sessionID, u.ID.String())
	if err := tx.Commit(r.Context()); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create session.")
		return
	}

	uid := u.ID
	s.auth.Audit(r.Context(), &uid, "rmm.session.create", "rmm_session", sessionID.String(), "ok", r.RemoteAddr,
		map[string]any{"host_id": hostID, "mode": "browser"})
	s.hub.Broadcast("rmm.session.updated", map[string]any{
		"session_id": sessionID, "host_id": hostID, "status": "pending", "mode": "browser",
	})

	sessionNote := rmmRDPConcurrentNote(concurrent, productType, osVersion)
	httpx.JSON(w, http.StatusCreated, map[string]any{
		"id":                      sessionID,
		"host_id":                 hostID,
		"mode":                    "browser",
		"status":                  "pending",
		"expires_at":              expires,
		"viewer_url":              fmt.Sprintf("/rmm/%s/session/%s", hostID, sessionID),
		"rdp_concurrent_sessions": concurrent,
		"rdp_session_note":        sessionNote,
	})
}

func (s *Server) handleCreateRMMNativeSession(w http.ResponseWriter, r *http.Request) {
	hostID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation", "Invalid host id.")
		return
	}
	var body struct {
		Confirm bool `json:"confirm"`
	}
	if err := httpx.Decode(r, &body); err != nil || !body.Confirm {
		httpx.Error(w, http.StatusBadRequest, "confirmation_required",
			"Starting a native RDP session requires confirm=true.")
		return
	}
	if rmmSessionLimiter != nil && !rmmSessionLimiter.allow(r.Context(), clientIP(r)) {
		httpx.Error(w, http.StatusTooManyRequests, "rate_limited", "Too many session attempts. Try again shortly.")
		return
	}

	u := r.Context().Value(ctxUser).(auth.User)
	var status string
	var rdpEnabled bool
	err = s.pool.QueryRow(r.Context(), `SELECT status, rdp_enabled FROM rmm_hosts WHERE id=$1`, hostID).
		Scan(&status, &rdpEnabled)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "RMM host not found.")
		return
	}
	if status != "online" {
		httpx.Error(w, http.StatusConflict, "host_offline", "Host is offline.")
		return
	}
	if !rdpEnabled {
		httpx.Error(w, http.StatusConflict, "rdp_disabled", "Remote Desktop appears disabled on the host.")
		return
	}
	if n := s.supersedeOpenRMMSessions(r.Context(), hostID, "superseded by new native session"); n > 0 {
		log.Printf("rmm: superseded %d open session(s) on host %s before native create", n, hostID)
		uid := u.ID
		s.auth.Audit(r.Context(), &uid, "rmm.session.supersede", "rmm_host", hostID.String(), "ok", r.RemoteAddr,
			map[string]any{"count": n, "mode": "native"})
	}

	tunnelToken, err := auth.NewToken(24)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create session.")
		return
	}
	nativeToken, err := auth.NewToken(24)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create session.")
		return
	}
	expires := time.Now().UTC().Add(4 * time.Hour)
	nativeExpires := time.Now().UTC().Add(2 * time.Minute)
	cmdExpires := time.Now().UTC().Add(2 * time.Minute)

	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create session.")
		return
	}
	defer tx.Rollback(r.Context())

	var sessionID uuid.UUID
	err = tx.QueryRow(r.Context(), `
		INSERT INTO rmm_sessions (
			host_id, mode, status, created_by, tunnel_token_hash,
			native_token_hash, native_token_expires_at, expires_at)
		VALUES ($1, 'native', 'pending', $2, $3, $4, $5, $6)
		RETURNING id`,
		hostID, u.ID, auth.HashToken(tunnelToken), auth.HashToken(nativeToken), nativeExpires, expires,
	).Scan(&sessionID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create session.")
		return
	}

	tunnelURL := strings.TrimRight(s.cfg.APIPublicURL, "/") + "/rmm/v1/tunnel"
	payload, _ := json.Marshal(map[string]any{
		"session_id":   sessionID,
		"tunnel_url":   tunnelURL,
		"tunnel_token": tunnelToken,
		"rdp_host":     "127.0.0.1",
		"mode":         "native",
	})
	_, err = tx.Exec(r.Context(), `
		INSERT INTO rmm_commands (host_id, type, payload, expires_at)
		VALUES ($1, 'session.create', $2::jsonb, $3)`, hostID, string(payload), cmdExpires)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not queue session command.")
		return
	}
	_, _ = tx.Exec(r.Context(), `
		INSERT INTO rmm_session_events (session_id, kind, message, context)
		VALUES ($1, 'session.created', 'Native RDP session requested.', jsonb_build_object('user_id', $2::text))`,
		sessionID, u.ID.String())
	if err := tx.Commit(r.Context()); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create session.")
		return
	}

	uid := u.ID
	s.auth.Audit(r.Context(), &uid, "rmm.session.create", "rmm_session", sessionID.String(), "ok", r.RemoteAddr,
		map[string]any{"host_id": hostID, "mode": "native"})
	s.hub.Broadcast("rmm.session.updated", map[string]any{
		"session_id": sessionID, "host_id": hostID, "status": "pending", "mode": "native",
	})

	gwHost := s.cfg.RMMNativePublicHost
	gwPort := s.cfg.RMMNativePublicPort
	rdp := buildNativeRDPFile(gwHost, gwPort, sessionID.String())
	launcher := buildNativeLauncherPS1(gwHost, gwPort, nativeToken)

	httpx.JSON(w, http.StatusCreated, map[string]any{
		"id":                   sessionID,
		"host_id":              hostID,
		"mode":                 "native",
		"status":               "pending",
		"expires_at":           expires,
		"gateway_host":         gwHost,
		"gateway_port":         gwPort,
		"token":                nativeToken,
		"token_expires_at":     nativeExpires,
		"rdp":                  rdp,
		"launcher_ps1":         launcher,
		"note":                 "Run the launcher (or fleetdeck-rmm connect) within 2 minutes. It authenticates to the gateway then opens Remote Desktop to a local forward.",
	})
}

func (s *Server) handleGetRMMSession(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "sessionID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation", "Invalid session id.")
		return
	}
	var sess struct {
		ID        uuid.UUID  `json:"id"`
		HostID    uuid.UUID  `json:"host_id"`
		Mode      string     `json:"mode"`
		Status    string     `json:"status"`
		CreatedBy *uuid.UUID `json:"created_by"`
		Error     string     `json:"error"`
		StartedAt *time.Time `json:"started_at"`
		EndedAt   *time.Time `json:"ended_at"`
		ExpiresAt time.Time  `json:"expires_at"`
		CreatedAt time.Time  `json:"created_at"`
	}
	err = s.pool.QueryRow(r.Context(), `
		SELECT id, host_id, mode, status, created_by, error, started_at, ended_at, expires_at, created_at
		FROM rmm_sessions WHERE id=$1`, id,
	).Scan(&sess.ID, &sess.HostID, &sess.Mode, &sess.Status, &sess.CreatedBy, &sess.Error,
		&sess.StartedAt, &sess.EndedAt, &sess.ExpiresAt, &sess.CreatedAt)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "Session not found.")
		return
	}
	httpx.JSON(w, http.StatusOK, sess)
}

func (s *Server) handleDisconnectRMMSession(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "sessionID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation", "Invalid session id.")
		return
	}
	var body struct {
		Confirm bool `json:"confirm"`
	}
	if err := httpx.Decode(r, &body); err != nil || !body.Confirm {
		httpx.Error(w, http.StatusBadRequest, "confirmation_required",
			"Disconnect requires confirm=true.")
		return
	}
	var hostID uuid.UUID
	err = s.pool.QueryRow(r.Context(), `SELECT host_id FROM rmm_sessions WHERE id=$1`, id).
		Scan(&hostID)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "Session not found.")
		return
	}
	s.closeRMMSession(r.Context(), id, hostID, "session.disconnect", "Session disconnected from panel.", "")

	u := r.Context().Value(ctxUser).(auth.User)
	uid := u.ID
	s.auth.Audit(r.Context(), &uid, "rmm.session.disconnect", "rmm_session", id.String(), "ok", r.RemoteAddr,
		map[string]any{"host_id": hostID})
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleRMMGuacTunnel(w http.ResponseWriter, r *http.Request) {
	u, err := s.auth.UserFromRequest(r.Context(), r)
	if err != nil {
		httpx.Error(w, http.StatusUnauthorized, "unauthenticated", "Sign in required.")
		return
	}
	if !roleAllowed(u.Role, "admin", "operator") {
		httpx.Error(w, http.StatusForbidden, "forbidden", "Your role cannot perform this action.")
		return
	}
	origin := r.Header.Get("Origin")
	if !realtimeOriginOK(origin, s.cfg.WebOrigins, s.cfg.CookieSecure) {
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}
	sessionID, err := uuid.Parse(chi.URLParam(r, "sessionID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation", "Invalid session id.")
		return
	}
	var status string
	var hostID uuid.UUID
	err = s.pool.QueryRow(r.Context(), `SELECT host_id, status FROM rmm_sessions WHERE id=$1`, sessionID).
		Scan(&hostID, &status)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "Session not found.")
		return
	}
	if status == "closed" || status == "failed" {
		httpx.Error(w, http.StatusConflict, "session_closed", "Session is no longer active.")
		return
	}
	st := s.rmmHub.Get(sessionID)
	if st == nil || st.ListenAddr == "" {
		httpx.Error(w, http.StatusConflict, "tunnel_not_ready",
			"Agent tunnel is not ready yet. Wait a few seconds and retry.")
		return
	}
	tunnelHost, tunnelPort, err := net.SplitHostPort(st.ListenAddr)
	if err != nil || tunnelHost == "" || tunnelPort == "" {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Tunnel listen address is invalid.")
		return
	}

	guacAddr := s.cfg.GuacdAddr
	if guacAddr == "" {
		guacAddr = "127.0.0.1:4822"
	}
	guacConn, err := net.DialTimeout("tcp", guacAddr, 5*time.Second)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "guacd_unavailable",
			"Could not reach guacd. Ensure the guacd Compose service is running.")
		return
	}

	// guacamole-common-js requests the "guacamole" subprotocol; echo it or the browser aborts.
	hdr := http.Header{}
	if p := r.Header.Get("Sec-Websocket-Protocol"); p != "" {
		hdr.Set("Sec-Websocket-Protocol", p)
	}
	ws, err := rmmWSUpgrader.Upgrade(w, r, hdr)
	if err != nil {
		_ = guacConn.Close()
		return
	}
	_, _ = s.pool.Exec(r.Context(), `
		INSERT INTO rmm_session_events (session_id, kind, message)
		VALUES ($1, 'guac.connected', 'Browser Guacamole viewer connected.')`, sessionID)
	// Prefer live DB status (tunnel attach may have flipped pending→active after we read above).
	_ = s.pool.QueryRow(r.Context(), `SELECT status FROM rmm_sessions WHERE id=$1`, sessionID).Scan(&status)
	s.hub.Broadcast("rmm.session.updated", map[string]any{
		"session_id": sessionID, "host_id": hostID, "status": status, "viewer": "connected",
	})

	// Browser may send display params only. Force tunnel target + credentials server-side so
	// clients cannot SSRF via guacd or put passwords in the WebSocket URL.
	q := sanitizeGuacDisplayParams(r.URL.Query())
	q.Set("hostname", tunnelHost)
	q.Set("port", tunnelPort)
	s.applyRMMRDPCredentials(r.Context(), hostID, sessionID, q)
	bg := context.Background()
	if err := rmm.PipeWSToGuacd(ws, guacConn, q); err != nil {
		log.Printf("rmm guac session %s: %v", sessionID, err)
		_, _ = s.pool.Exec(bg, `
			INSERT INTO rmm_session_events (session_id, kind, message, context)
			VALUES ($1, 'guac.error', $2, '{}'::jsonb)`, sessionID, err.Error())
		// Auth failures leave the agent tunnel up unless we close here — otherwise
		// Connect stays blocked by an orphan active/pending row.
		s.closeRMMSession(bg, sessionID, hostID, "guac.failed",
			"Browser Guacamole viewer failed; session closed.", err.Error())
		return
	}

	_, _ = s.pool.Exec(bg, `
		INSERT INTO rmm_session_events (session_id, kind, message)
		VALUES ($1, 'guac.disconnected', 'Browser Guacamole viewer disconnected.')`, sessionID)
	// Intentionally do not close on clean viewer disconnect: same-page Retry and
	// React remounts disconnect/reconnect the Guacamole client. Page unload and
	// Disconnect call the disconnect API; Connect auto-supersedes leftovers.
}

func (s *Server) handleRMMSessionConnectionInfo(w http.ResponseWriter, r *http.Request) {
	sessionID, err := uuid.Parse(chi.URLParam(r, "sessionID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation", "Invalid session id.")
		return
	}
	var hostID uuid.UUID
	err = s.pool.QueryRow(r.Context(), `SELECT host_id FROM rmm_sessions WHERE id=$1`, sessionID).Scan(&hostID)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "Session not found.")
		return
	}
	st := s.rmmHub.Get(sessionID)
	if st == nil {
		httpx.Error(w, http.StatusConflict, "tunnel_not_ready", "Agent tunnel is not ready yet.")
		return
	}
	host, portStr, _ := net.SplitHostPort(st.ListenAddr)
	port, _ := strconv.Atoi(portStr)
	out := map[string]any{
		"hostname":         host,
		"port":             port,
		"security":         "any",
		"ignore-cert":      true,
		"enable-wallpaper": true,
		"resize-method":    "display-update",
		"enable-drive":     false,
		"disable-audio":    false,
	}
	var concurrent bool
	var productType, osVersion string
	_ = s.pool.QueryRow(r.Context(), `
		SELECT rdp_concurrent_sessions, os_product_type, os_version FROM rmm_hosts WHERE id=$1`, hostID).
		Scan(&concurrent, &productType, &osVersion)
	out["rdp_concurrent_sessions"] = concurrent
	out["rdp_session_note"] = rmmRDPConcurrentNote(concurrent, productType, osVersion)
	if cred, ok := s.loadRMMRDPSecret(r.Context(), hostID); ok {
		// Username only — never return the RDP password to the browser.
		out["username"] = formatLocalRDPUsername(cred.Username)
		out["auto_login"] = true
	} else {
		var username string
		_ = s.pool.QueryRow(r.Context(), `SELECT rdp_username FROM rmm_hosts WHERE id=$1`, hostID).Scan(&username)
		if username != "" {
			out["username"] = formatLocalRDPUsername(username)
		}
		out["auto_login"] = false
	}
	httpx.JSON(w, http.StatusOK, out)
}

// handleSetRMMSessionRDPCredentials accepts operator-pasted RDP credentials over HTTPS JSON
// (not WebSocket query strings), stores them for the host, and scopes an override to this session.
func (s *Server) handleSetRMMSessionRDPCredentials(w http.ResponseWriter, r *http.Request) {
	sessionID, err := uuid.Parse(chi.URLParam(r, "sessionID"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation", "Invalid session id.")
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "Invalid request body.")
		return
	}
	body.Username = strings.TrimSpace(body.Username)
	if body.Username == "" || body.Password == "" {
		httpx.Error(w, http.StatusBadRequest, "validation", "Username and password are required.")
		return
	}
	var hostID uuid.UUID
	var status string
	err = s.pool.QueryRow(r.Context(), `SELECT host_id, status FROM rmm_sessions WHERE id=$1`, sessionID).
		Scan(&hostID, &status)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "Session not found.")
		return
	}
	if status == "closed" || status == "failed" {
		httpx.Error(w, http.StatusConflict, "session_closed", "Session is no longer active.")
		return
	}
	if err := s.storeRMMRDPSecret(r.Context(), hostID, body.Username, body.Password); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not store RDP credentials.")
		return
	}
	bare := body.Username
	bare = strings.TrimPrefix(bare, `.\`)
	if i := strings.LastIndexAny(bare, `/\`); i >= 0 {
		bare = bare[i+1:]
	}
	_, _ = s.pool.Exec(r.Context(), `
		UPDATE rmm_hosts SET rdp_username=$2, updated_at=now() WHERE id=$1`, hostID, bare)
	s.setRMMSessionCredOverride(sessionID, body.Username, body.Password)

	u := r.Context().Value(ctxUser).(auth.User)
	uid := u.ID
	s.auth.Audit(r.Context(), &uid, "rmm.session.rdp_credentials", "rmm_session", sessionID.String(), "ok", r.RemoteAddr,
		map[string]any{"host_id": hostID})
	httpx.JSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"username":   formatLocalRDPUsername(bare),
		"auto_login": true,
	})
}

func buildNativeRDPFile(host string, port int, title string) string {
	return fmt.Sprintf(`full address:s:%s:%d
prompt for credentials:i:1
administrative session:i:0
username:s:
desktopwidth:i:1280
desktopheight:i:800
session bpp:i:32
compression:i:1
authentication level:i:2
negotiate security layer:i:1
bitmapcachepersistenable:i:1
audiomode:i:0
redirectclipboard:i:1
redirectprinters:i:0
redirectcomports:i:0
redirectsmartcards:i:0
redirectdrives:i:0
redirectposdevices:i:0
autoreconnection enabled:i:1
screen mode id:i:1
use multimon:i:0
`, host, port)
}


func buildNativeLauncherPS1(host string, port int, token string) string {
	preview := token
	if len(preview) > 8 {
		preview = preview[:8]
	}
	return fmt.Sprintf(
		"# FleetDeck native RDP\n#   fleetdeck-rmm -connect %s:%d -connect-token %s\n# Then open Remote Desktop to 127.0.0.1:13389\nWrite-Host 'Use fleetdeck-rmm -connect %s:%d -connect-token <panel-token>'\nWrite-Host 'Gateway %s:%d token prefix: %s...'\n",
		host, port, token, host, port, host, port, preview,
	)
}

