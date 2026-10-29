package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fleetdeck/fleetdeck/apps/api/internal/auth"
	"github.com/fleetdeck/fleetdeck/apps/api/internal/httpx"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const rmmAgentVersionDefault = "0.1.4-dev"

// rmmRDPConcurrentNote explains Windows SKU limits for FleetDeck Connect.
// Client Windows cannot host a second interactive desktop without takeover prompts;
// Server SKUs can (admin RDP / RDSH). FleetDeck never kicks local users.
func rmmRDPConcurrentNote(concurrent bool, productType, osVersion string) string {
	if concurrent || productType == "server" || productType == "domain_controller" {
		return "Windows Server can host concurrent RDP sessions alongside a console user (within admin/RDS limits)."
	}
	ver := strings.ToLower(osVersion)
	if strings.Contains(ver, "server") {
		return "Windows Server can host concurrent RDP sessions alongside a console user (within admin/RDS limits). Re-run install-rmm.ps1 to apply fSingleSessionPerUser=0."
	}
	return "Windows Pro/client allows only one interactive desktop. FleetDeck will not disconnect the local user — if someone is logged on at the console, Windows will show takeover / consent prompts. Concurrent sessions without kicking require Windows Server."
}

type rmmHostIdentity struct {
	HostID uuid.UUID
}

var rmmWSUpgrader = websocket.Upgrader{
	ReadBufferSize:  32 * 1024,
	WriteBufferSize: 32 * 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func (s *Server) requireRMMAgent(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		publicID, secret, ok := parseAgentBearer(r.Header.Get("Authorization"))
		if !ok {
			httpx.Error(w, http.StatusUnauthorized, "agent_unauthenticated", "RMM agent credentials required.")
			return
		}
		var hostID uuid.UUID
		var secretHash string
		err := s.pool.QueryRow(r.Context(), `
			SELECT c.host_id, c.secret_hash
			FROM rmm_credentials c
			WHERE c.public_id=$1 AND c.revoked_at IS NULL`, publicID,
		).Scan(&hostID, &secretHash)
		if err != nil || !auth.VerifySecret(secretHash, secret) {
			httpx.Error(w, http.StatusUnauthorized, "agent_unauthenticated", "Invalid RMM agent credentials.")
			return
		}
		ctx := context.WithValue(r.Context(), ctxRMMHost, rmmHostIdentity{HostID: hostID})
		next(w, r.WithContext(ctx))
	}
}

const ctxRMMHost ctxKey = "rmm_host"

// --- Panel: hosts & enrollment ---

func (s *Server) handleListRMMHosts(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT id, name, hostname, os_name, os_version, arch, agent_version, status,
		       rdp_enabled, rdp_port, rdp_username, os_edition, os_product_type, rdp_concurrent_sessions,
		       last_heartbeat_at, last_seen_at, server_id, created_at
		FROM rmm_hosts
		ORDER BY name ASC`)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list RMM hosts.")
		return
	}
	defer rows.Close()
	type row struct {
		ID                     uuid.UUID  `json:"id"`
		Name                   string     `json:"name"`
		Hostname               string     `json:"hostname"`
		OSName                 string     `json:"os_name"`
		OSVersion              string     `json:"os_version"`
		Arch                   string     `json:"arch"`
		AgentVersion           string     `json:"agent_version"`
		Status                 string     `json:"status"`
		RDPEnabled             bool       `json:"rdp_enabled"`
		RDPPort                int        `json:"rdp_port"`
		RDPUsername            string     `json:"rdp_username"`
		OSEdition              string     `json:"os_edition"`
		OSProductType          string     `json:"os_product_type"`
		RDPConcurrentSessions  bool       `json:"rdp_concurrent_sessions"`
		HasRDPCredentials      bool       `json:"has_rdp_credentials"`
		LastHeartbeatAt        *time.Time `json:"last_heartbeat_at"`
		LastSeenAt             *time.Time `json:"last_seen_at"`
		ServerID               *uuid.UUID `json:"server_id"`
		CreatedAt              time.Time  `json:"created_at"`
	}
	out := make([]row, 0)
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.ID, &x.Name, &x.Hostname, &x.OSName, &x.OSVersion, &x.Arch, &x.AgentVersion,
			&x.Status, &x.RDPEnabled, &x.RDPPort, &x.RDPUsername, &x.OSEdition, &x.OSProductType, &x.RDPConcurrentSessions,
			&x.LastHeartbeatAt, &x.LastSeenAt, &x.ServerID, &x.CreatedAt); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not list RMM hosts.")
			return
		}
		x.HasRDPCredentials = s.hasRMMRDPSecret(r.Context(), x.ID) && x.RDPUsername != ""
		out = append(out, x)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"data":                out,
		"current_rmm_version": s.cfg.RMMAgentVersion,
	})
}

func (s *Server) handleGetRMMHost(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation", "Invalid host id.")
		return
	}
	var x struct {
		ID                    uuid.UUID  `json:"id"`
		Name                  string     `json:"name"`
		Hostname              string     `json:"hostname"`
		OSName                string     `json:"os_name"`
		OSVersion             string     `json:"os_version"`
		Arch                  string     `json:"arch"`
		AgentVersion          string     `json:"agent_version"`
		Status                string     `json:"status"`
		RDPEnabled            bool       `json:"rdp_enabled"`
		RDPPort               int        `json:"rdp_port"`
		RDPUsername           string     `json:"rdp_username"`
		OSEdition             string     `json:"os_edition"`
		OSProductType         string     `json:"os_product_type"`
		RDPConcurrentSessions bool       `json:"rdp_concurrent_sessions"`
		HasRDPCredentials     bool       `json:"has_rdp_credentials"`
		LastHeartbeatAt       *time.Time `json:"last_heartbeat_at"`
		LastSeenAt            *time.Time `json:"last_seen_at"`
		ServerID              *uuid.UUID `json:"server_id"`
		CreatedAt             time.Time  `json:"created_at"`
	}
	err = s.pool.QueryRow(r.Context(), `
		SELECT id, name, hostname, os_name, os_version, arch, agent_version, status,
		       rdp_enabled, rdp_port, rdp_username, os_edition, os_product_type, rdp_concurrent_sessions,
		       last_heartbeat_at, last_seen_at, server_id, created_at
		FROM rmm_hosts WHERE id=$1`, id,
	).Scan(&x.ID, &x.Name, &x.Hostname, &x.OSName, &x.OSVersion, &x.Arch, &x.AgentVersion,
		&x.Status, &x.RDPEnabled, &x.RDPPort, &x.RDPUsername, &x.OSEdition, &x.OSProductType, &x.RDPConcurrentSessions,
		&x.LastHeartbeatAt, &x.LastSeenAt, &x.ServerID, &x.CreatedAt)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "RMM host not found.")
		return
	}
	x.HasRDPCredentials = s.hasRMMRDPSecret(r.Context(), x.ID) && x.RDPUsername != ""

	var openSession map[string]any
	var sessID uuid.UUID
	var sessMode, sessStatus string
	var sessCreatedAt time.Time
	err = s.pool.QueryRow(r.Context(), `
		SELECT id, mode, status, created_at
		FROM rmm_sessions
		WHERE host_id=$1 AND status IN ('pending','connecting','active')
		ORDER BY created_at DESC
		LIMIT 1`, id).Scan(&sessID, &sessMode, &sessStatus, &sessCreatedAt)
	if err == nil {
		openSession = map[string]any{
			"id":         sessID,
			"mode":       sessMode,
			"status":     sessStatus,
			"created_at": sessCreatedAt,
			"viewer_url": fmt.Sprintf("/rmm/%s/session/%s", id, sessID),
		}
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"id":                      x.ID,
		"name":                    x.Name,
		"hostname":                x.Hostname,
		"os_name":                 x.OSName,
		"os_version":              x.OSVersion,
		"arch":                    x.Arch,
		"agent_version":           x.AgentVersion,
		"status":                  x.Status,
		"rdp_enabled":             x.RDPEnabled,
		"rdp_port":                x.RDPPort,
		"rdp_username":            x.RDPUsername,
		"os_edition":              x.OSEdition,
		"os_product_type":         x.OSProductType,
		"rdp_concurrent_sessions": x.RDPConcurrentSessions,
		"rdp_session_note":        rmmRDPConcurrentNote(x.RDPConcurrentSessions, x.OSProductType, x.OSVersion),
		"has_rdp_credentials":     x.HasRDPCredentials,
		"last_heartbeat_at":       x.LastHeartbeatAt,
		"last_seen_at":            x.LastSeenAt,
		"server_id":               x.ServerID,
		"created_at":              x.CreatedAt,
		"current_rmm_version":     s.cfg.RMMAgentVersion,
		"open_session":            openSession,
	})
}

func (s *Server) handleCreateRMMEnrollmentToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Label      string `json:"label"`
		NameHint   string `json:"name_hint"`
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
	if body.TTLMinutes > 24*60 {
		body.TTLMinutes = 24 * 60
	}
	if body.MaxUses <= 0 {
		body.MaxUses = 1
	}
	if body.MaxUses > 50 {
		body.MaxUses = 50
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
		INSERT INTO rmm_enrollment_tokens (token_hash, label, name_hint, expires_at, max_uses, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`, auth.HashToken(token), body.Label, body.NameHint, expires, body.MaxUses, u.ID,
	).Scan(&id)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create enrollment token.")
		return
	}
	uid := u.ID
	s.auth.Audit(r.Context(), &uid, "rmm.enrollment_token.create", "rmm_enrollment_token", id.String(), "ok", r.RemoteAddr, nil)
	httpx.JSON(w, http.StatusCreated, map[string]any{
		"id":         id,
		"token":      token,
		"expires_at": expires,
		"max_uses":   body.MaxUses,
		"name_hint":  body.NameHint,
		"warning":    "Store this token securely. It will not be shown again.",
	})
}

func (s *Server) handleDeleteRMMHost(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation", "Invalid host id.")
		return
	}
	var body struct {
		Confirm bool `json:"confirm"`
	}
	if err := httpx.Decode(r, &body); err != nil || !body.Confirm {
		httpx.Error(w, http.StatusBadRequest, "confirmation_required",
			"Removing an RMM host requires confirm=true.")
		return
	}
	tag, err := s.pool.Exec(r.Context(), `DELETE FROM rmm_hosts WHERE id=$1`, id)
	if err != nil || tag.RowsAffected() == 0 {
		httpx.Error(w, http.StatusNotFound, "not_found", "RMM host not found.")
		return
	}
	if s.secrets != nil {
		_ = s.secrets.Delete(r.Context(), "rmm_rdp", id.String())
	}
	u := r.Context().Value(ctxUser).(auth.User)
	uid := u.ID
	s.auth.Audit(r.Context(), &uid, "rmm.host.delete", "rmm_host", id.String(), "ok", r.RemoteAddr, nil)
	s.hub.Broadcast("rmm.hosts.updated", map[string]any{"reason": "deleted", "host_id": id})
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleUpdateRMMAgent queues agent.update so an online Windows RMM host pulls a new binary from CDN and restarts.
func (s *Server) handleUpdateRMMAgent(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(ctxUser).(auth.User)
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation", "Invalid host id.")
		return
	}
	var body struct {
		Confirm bool   `json:"confirm"`
		Channel string `json:"channel"`
	}
	if err := httpx.Decode(r, &body); err != nil || !body.Confirm {
		httpx.Error(w, http.StatusBadRequest, "confirmation_required",
			"Updating an RMM agent requires confirm=true.")
		return
	}

	var name, status, agentVersion string
	var lastHeartbeat *time.Time
	err = s.pool.QueryRow(r.Context(), `
		SELECT name, status, COALESCE(agent_version, ''), last_heartbeat_at
		FROM rmm_hosts WHERE id=$1`, id,
	).Scan(&name, &status, &agentVersion, &lastHeartbeat)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "RMM host not found.")
		return
	}
	if !agentReachable(status, lastHeartbeat) {
		httpx.ErrorDetails(w, http.StatusConflict, "agent_offline",
			"RMM agent appears offline. Wait until it is online, then retry Update agent.",
			map[string]any{"host_id": id, "status": status, "wait_required": true}, nil)
		return
	}

	cdnBase := strings.TrimRight(s.cfg.AgentCDNBase, "/")
	if cdnBase == "" {
		cdnBase = "https://cdn.example.com/fleetdeck"
	}
	channel := strings.TrimSpace(body.Channel)
	if channel == "" {
		channel = s.cfg.AgentCDNChannel
	}
	if channel == "" {
		channel = "latest"
	}
	if !validCDNChannel(channel) {
		httpx.Error(w, http.StatusBadRequest, "validation",
			"Invalid CDN channel. Use letters, digits, '.', '_', or '-' only.")
		return
	}

	payload, _ := json.Marshal(map[string]any{
		"cdn_base": cdnBase,
		"channel":  channel,
		"url":      cdnBase + "/" + channel,
	})
	var cmdID uuid.UUID
	err = s.pool.QueryRow(r.Context(), `
		INSERT INTO rmm_commands (host_id, type, payload, expires_at)
		VALUES ($1, 'agent.update', $2::jsonb, now() + interval '3 minutes')
		RETURNING id`, id, string(payload),
	).Scan(&cmdID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not enqueue RMM agent update.")
		return
	}

	uid := u.ID
	s.auth.Audit(r.Context(), &uid, "rmm.agent.update", "rmm_host", id.String(), "queued", r.RemoteAddr, map[string]any{
		"name": name, "command_id": cmdID, "cdn_base": cdnBase, "channel": channel,
	})

	cmdStatus, result, errText, timedOut := s.waitRMMCommand(r, cmdID, 90*time.Second)
	if timedOut {
		httpx.ErrorDetails(w, http.StatusGatewayTimeout, "update_timeout",
			"RMM agent did not finish staging the update in time. Check the Windows service logs, then retry.",
			map[string]any{"command_id": cmdID, "host_id": id, "cdn": cdnBase + "/" + channel}, nil)
		return
	}
	if cmdStatus != "completed" {
		msg := "RMM agent update failed."
		if errText != "" {
			msg = errText
		}
		s.auth.Audit(r.Context(), &uid, "rmm.agent.update", "rmm_host", id.String(), "failed", r.RemoteAddr, map[string]any{
			"error": msg, "command_id": cmdID,
		})
		httpx.ErrorDetails(w, http.StatusBadGateway, "update_failed", msg,
			map[string]any{"command_id": cmdID, "result": result}, nil)
		return
	}

	s.auth.Audit(r.Context(), &uid, "rmm.agent.update", "rmm_host", id.String(), "ok", r.RemoteAddr, map[string]any{
		"command_id": cmdID, "previous_version": agentVersion,
	})
	if s.hub != nil {
		s.hub.Broadcast("rmm.hosts.updated", map[string]any{"host_id": id, "agent_update": true})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"ok":               true,
		"id":               id,
		"name":             name,
		"command_id":       cmdID,
		"result":           result,
		"channel":          channel,
		"cdn_base":         cdnBase,
		"previous_version": agentVersion,
		"message":          "Update staged; RMM agent is restarting with the new binary. Version refreshes on next heartbeat.",
	})
}

func (s *Server) waitRMMCommand(r *http.Request, cmdID uuid.UUID, timeout time.Duration) (status, result, errText string, timedOut bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var res, errPtr *string
		err := s.pool.QueryRow(r.Context(), `
			SELECT status, result_text, NULLIF(error,'') FROM rmm_commands WHERE id=$1`, cmdID,
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

// --- Agent: enroll / heartbeat / commands / tunnel ---

func (s *Server) handleRMMEnroll(w http.ResponseWriter, r *http.Request) {
	if enrollLimiter != nil && !enrollLimiter.allow(r.Context(), clientIP(r)) {
		httpx.Error(w, http.StatusTooManyRequests, "rate_limited", "Too many enrollment attempts. Try again shortly.")
		return
	}
	var body struct {
		Token                  string `json:"token"`
		AgentVersion           string `json:"agent_version"`
		Hostname               string `json:"hostname"`
		OSName                 string `json:"os_name"`
		OSVersion              string `json:"os_version"`
		Arch                   string `json:"arch"`
		RDPEnabled             bool   `json:"rdp_enabled"`
		RDPPort                int    `json:"rdp_port"`
		Name                   string `json:"name"`
		RDPUsername            string `json:"rdp_username"`
		RDPPassword            string `json:"rdp_password"`
		OSEdition              string `json:"os_edition"`
		OSProductType          string `json:"os_product_type"`
		RDPConcurrentSessions  *bool  `json:"rdp_concurrent_sessions"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "Invalid request body.")
		return
	}
	if strings.TrimSpace(body.Token) == "" {
		httpx.Error(w, http.StatusBadRequest, "validation", "Enrollment token is required.")
		return
	}
	if body.RDPPort <= 0 {
		body.RDPPort = 3389
	}
	if body.OSName == "" {
		body.OSName = "windows"
	}
	if body.Arch == "" {
		body.Arch = "amd64"
	}
	if body.AgentVersion == "" {
		body.AgentVersion = rmmAgentVersionDefault
	}

	publicID, err := auth.NewToken(16)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not enroll RMM agent.")
		return
	}
	secret, err := auth.NewToken(32)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not enroll RMM agent.")
		return
	}
	secretHash, err := auth.HashSecret(secret)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not enroll RMM agent.")
		return
	}

	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not enroll RMM agent.")
		return
	}
	defer tx.Rollback(r.Context())

	// Atomically claim one use to prevent TOCTOU races on max_uses.
	var nameHint string
	err = tx.QueryRow(r.Context(), `
		UPDATE rmm_enrollment_tokens
		SET uses = uses + 1
		WHERE token_hash=$1 AND revoked_at IS NULL
		  AND expires_at > now() AND uses < max_uses
		RETURNING name_hint`, auth.HashToken(body.Token),
	).Scan(&nameHint)
	if err != nil {
		httpx.Error(w, http.StatusUnauthorized, "invalid_enrollment_token", "Enrollment token is invalid or expired.")
		return
	}

	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = strings.TrimSpace(nameHint)
	}
	if name == "" {
		name = strings.TrimSpace(body.Hostname)
	}
	if name == "" {
		name = "windows-host"
	}

	concurrent := false
	if body.RDPConcurrentSessions != nil {
		concurrent = *body.RDPConcurrentSessions
	} else if body.OSProductType == "server" || body.OSProductType == "domain_controller" {
		concurrent = true
	}

	var hostID uuid.UUID
	err = tx.QueryRow(r.Context(), `
		INSERT INTO rmm_hosts (name, hostname, os_name, os_version, arch, agent_version, status,
		                       rdp_enabled, rdp_port, rdp_username, os_edition, os_product_type,
		                       rdp_concurrent_sessions, last_heartbeat_at, last_seen_at)
		VALUES ($1,$2,$3,$4,$5,$6,'online',$7,$8,$9,$10,$11,$12,now(),now())
		RETURNING id`,
		name, body.Hostname, body.OSName, body.OSVersion, body.Arch, body.AgentVersion,
		body.RDPEnabled, body.RDPPort, strings.TrimSpace(body.RDPUsername),
		strings.TrimSpace(body.OSEdition), strings.TrimSpace(body.OSProductType), concurrent,
	).Scan(&hostID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not enroll RMM agent.")
		return
	}
	_, err = tx.Exec(r.Context(), `
		INSERT INTO rmm_credentials (host_id, public_id, secret_hash)
		VALUES ($1, $2, $3)`, hostID, publicID, secretHash)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not enroll RMM agent.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not enroll RMM agent.")
		return
	}

	if strings.TrimSpace(body.RDPUsername) != "" && strings.TrimSpace(body.RDPPassword) != "" {
		if err := s.storeRMMRDPSecret(r.Context(), hostID, body.RDPUsername, body.RDPPassword); err != nil {
			log.Printf("rmm enroll: failed to store RDP secret for host %s: %v", hostID, err)
		}
	}

	s.hub.Broadcast("rmm.hosts.updated", map[string]any{"reason": "enrolled", "host_id": hostID})
	httpx.JSON(w, http.StatusCreated, map[string]any{
		"host_id":        hostID,
		"public_id":      publicID,
		"secret":         secret,
		"api_public_url": s.cfg.APIPublicURL,
		"warning":        "Store RMM agent credentials securely. The secret will not be shown again.",
	})
}

func (s *Server) handleRMMHeartbeat(w http.ResponseWriter, r *http.Request) {
	ident := r.Context().Value(ctxRMMHost).(rmmHostIdentity)
	var body struct {
		AgentVersion          string `json:"agent_version"`
		Hostname              string `json:"hostname"`
		OSVersion             string `json:"os_version"`
		RDPEnabled            *bool  `json:"rdp_enabled"`
		RDPPort               int    `json:"rdp_port"`
		RDPUsername           string `json:"rdp_username"`
		RDPPassword           string `json:"rdp_password"`
		OSEdition             string `json:"os_edition"`
		OSProductType         string `json:"os_product_type"`
		RDPConcurrentSessions *bool  `json:"rdp_concurrent_sessions"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "Invalid heartbeat body.")
		return
	}
	rdpEnabled := false
	if body.RDPEnabled != nil {
		rdpEnabled = *body.RDPEnabled
	}
	if body.RDPPort <= 0 {
		body.RDPPort = 3389
	}
	concurrent := false
	if body.RDPConcurrentSessions != nil {
		concurrent = *body.RDPConcurrentSessions
	} else if body.OSProductType == "server" || body.OSProductType == "domain_controller" {
		concurrent = true
	}
	_, err := s.pool.Exec(r.Context(), `
		UPDATE rmm_hosts
		SET status='online',
		    agent_version=COALESCE(NULLIF($2,''), agent_version),
		    hostname=COALESCE(NULLIF($3,''), hostname),
		    os_version=COALESCE(NULLIF($4,''), os_version),
		    rdp_enabled=$5,
		    rdp_port=$6,
		    rdp_username=COALESCE(NULLIF($7,''), rdp_username),
		    os_edition=COALESCE(NULLIF($8,''), os_edition),
		    os_product_type=COALESCE(NULLIF($9,''), os_product_type),
		    rdp_concurrent_sessions=CASE WHEN $10 OR $9 IN ('server','domain_controller') THEN true
		                                 WHEN NULLIF($9,'') IS NOT NULL THEN false
		                                 ELSE rdp_concurrent_sessions END,
		    last_heartbeat_at=now(),
		    last_seen_at=now(),
		    updated_at=now()
		WHERE id=$1`,
		ident.HostID, body.AgentVersion, body.Hostname, body.OSVersion, rdpEnabled, body.RDPPort,
		strings.TrimSpace(body.RDPUsername), strings.TrimSpace(body.OSEdition),
		strings.TrimSpace(body.OSProductType), concurrent,
	)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not update heartbeat.")
		return
	}
	if strings.TrimSpace(body.RDPUsername) != "" && strings.TrimSpace(body.RDPPassword) != "" {
		if err := s.storeRMMRDPSecret(r.Context(), ident.HostID, body.RDPUsername, body.RDPPassword); err != nil {
			log.Printf("rmm heartbeat: failed to store RDP secret for host %s: %v", ident.HostID, err)
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleRMMPollCommands(w http.ResponseWriter, r *http.Request) {
	ident := r.Context().Value(ctxRMMHost).(rmmHostIdentity)
	_, _ = s.pool.Exec(r.Context(), `
		UPDATE rmm_commands SET status='expired'
		WHERE host_id=$1 AND status='pending' AND expires_at < now()`, ident.HostID)

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
			FROM rmm_commands
			WHERE host_id=$1 AND status='pending'
			ORDER BY created_at ASC
			LIMIT 10`, ident.HostID)
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
				_, _ = s.pool.Exec(r.Context(), `UPDATE rmm_commands SET status='running' WHERE id=$1 AND status='pending'`, id)
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

func (s *Server) handleRMMCommandResult(w http.ResponseWriter, r *http.Request) {
	ident := r.Context().Value(ctxRMMHost).(rmmHostIdentity)
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
	tag, err := s.pool.Exec(r.Context(), `
		UPDATE rmm_commands
		SET status=$3, result_text=$4, error=$5, completed_at=now()
		WHERE id=$1 AND host_id=$2 AND status IN ('pending','running')`,
		id, ident.HostID, status, body.Result, body.Error,
	)
	if err != nil || tag.RowsAffected() == 0 {
		httpx.Error(w, http.StatusNotFound, "not_found", "Command not found.")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleRMMAgentTunnel(w http.ResponseWriter, r *http.Request) {
	ident := r.Context().Value(ctxRMMHost).(rmmHostIdentity)
	sessionID, err := uuid.Parse(r.URL.Query().Get("session_id"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation", "session_id required.")
		return
	}
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Tunnel token required.")
		return
	}
	var hostID uuid.UUID
	var status string
	var tokenHash *string
	err = s.pool.QueryRow(r.Context(), `
		SELECT host_id, status, tunnel_token_hash
		FROM rmm_sessions WHERE id=$1`, sessionID,
	).Scan(&hostID, &status, &tokenHash)
	if err != nil || hostID != ident.HostID {
		httpx.Error(w, http.StatusNotFound, "not_found", "Session not found.")
		return
	}
	if tokenHash == nil || *tokenHash == "" || auth.HashToken(token) != *tokenHash {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Invalid tunnel token.")
		return
	}
	if status == "closed" || status == "failed" {
		httpx.Error(w, http.StatusConflict, "session_closed", "Session is no longer active.")
		return
	}

	conn, err := rmmWSUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	st, err := s.rmmHub.AttachAgent(sessionID, hostID, conn, s.cfg.RMMAdvertiseHost)
	if err != nil {
		_ = conn.Close()
		return
	}
	port := 0
	if parts := strings.Split(st.ListenAddr, ":"); len(parts) > 0 {
		port, _ = strconv.Atoi(parts[len(parts)-1])
	}
	_, _ = s.pool.Exec(context.Background(), `
		UPDATE rmm_sessions
		SET status='active', gateway_listen_port=$2, started_at=COALESCE(started_at, now()), updated_at=now()
		WHERE id=$1`, sessionID, port)
	_, _ = s.pool.Exec(context.Background(), `
		INSERT INTO rmm_session_events (session_id, kind, message)
		VALUES ($1, 'tunnel.attached', 'Agent reverse tunnel attached.')`, sessionID)
	s.hub.Broadcast("rmm.session.updated", map[string]any{
		"session_id": sessionID, "host_id": hostID, "status": "active",
	})

	// Block until tunnel closes (readAgentLoop closes on disconnect).
	<-st.Closed()
	s.rmmHub.Close(sessionID)
	s.clearRMMSessionCredOverride(sessionID)
	_, _ = s.pool.Exec(context.Background(), `
		UPDATE rmm_sessions
		SET status='closed', ended_at=now(), updated_at=now()
		WHERE id=$1 AND status IN ('pending','connecting','active')`, sessionID)
	s.hub.Broadcast("rmm.session.updated", map[string]any{
		"session_id": sessionID, "host_id": hostID, "status": "closed",
	})
}
