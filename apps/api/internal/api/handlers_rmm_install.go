package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fleetdeck/fleetdeck/apps/api/internal/auth"
	"github.com/fleetdeck/fleetdeck/apps/api/internal/httpx"
	"github.com/google/uuid"
)

func (s *Server) handleInstallRMMScript(w http.ResponseWriter, r *http.Request) {
	cdn := strings.TrimRight(s.cfg.AgentCDNBase, "/")
	if cdn == "" {
		cdn = "https://cdn.example.com/fleetdeck"
	}
	apiURL := strings.TrimRight(s.cfg.APIPublicURL, "/")
	if apiURL == "" {
		apiURL = "https://agents.example.com"
	}

	scriptPath := filepath.Join(s.cfg.RMMDistDir, "install-rmm.ps1")
	raw, err := os.ReadFile(scriptPath)
	if err != nil {
		// Fallback: try repo-relative path for local runs.
		raw, err = os.ReadFile(filepath.Join("cdn", "fleetdeck", "install-rmm.ps1"))
	}
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found",
			"install-rmm.ps1 not found on API. Rebuild the API image or publish CDN.")
		return
	}
	text := string(raw)
	text = strings.ReplaceAll(text, "https://cdn.example.com/fleetdeck", cdn)
	text = strings.ReplaceAll(text, "https://agents.example.com", apiURL)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", `inline; filename="install-rmm.ps1"`)
	_, _ = w.Write([]byte(text))
}

func (s *Server) handleInstallRMMBinary(w http.ResponseWriter, r *http.Request) {
	path := filepath.Join(s.cfg.RMMDistDir, "windows-amd64")
	if _, err := os.Stat(path); err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found",
			"RMM windows-amd64 binary not found on API. Rebuild the API image or publish CDN.")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="fleetdeck-rmm.exe"`)
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, path)
}

// handleRMMInstallCommand creates an enrollment token and returns a one-liner for elevated PowerShell.
func (s *Server) handleRMMInstallCommand(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Label      string `json:"label"`
		NameHint   string `json:"name_hint"`
		TTLMinutes int    `json:"ttl_minutes"`
		MaxUses    int    `json:"max_uses"`
		RdpUser    string `json:"rdp_user"`
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
	body.RdpUser = strings.TrimSpace(body.RdpUser)
	if body.RdpUser == "" {
		body.RdpUser = "fleetdeck"
	}
	if !validLocalRDPUser(body.RdpUser) {
		httpx.Error(w, http.StatusBadRequest, "validation",
			"rdp_user must be a simple local account name (letters, digits, '.', '_', '-'; max 20).")
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
		INSERT INTO rmm_enrollment_tokens (token_hash, label, name_hint, expires_at, max_uses, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`, auth.HashToken(token), body.Label, body.NameHint, expires, body.MaxUses, u.ID,
	).Scan(&id)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not create enrollment token.")
		return
	}

	apiURL := strings.TrimRight(s.cfg.APIPublicURL, "/")
	escToken := strings.ReplaceAll(token, "'", "''")
	escAPI := strings.ReplaceAll(apiURL, "'", "''")
	escUser := strings.ReplaceAll(body.RdpUser, "'", "''")

	// Save script to TEMP then invoke — avoids iex scriptblock error dumps and expansion pitfalls.
	command := fmt.Sprintf(
		`$__fd=Join-Path $env:TEMP ('fd-rmm-'+[guid]::NewGuid().ToString('n')+'.ps1'); Invoke-WebRequest -Uri '%s/install-rmm.ps1' -OutFile $__fd -UseBasicParsing; try { & $__fd -Token '%s' -ApiUrl '%s' -RdpUser '%s' } finally { Remove-Item -LiteralPath $__fd -Force -ErrorAction SilentlyContinue }`,
		apiURL, escToken, escAPI, escUser,
	)

	uid := u.ID
	s.auth.Audit(r.Context(), &uid, "rmm.install_command.create", "rmm_enrollment_token", id.String(), "ok", r.RemoteAddr, nil)

	httpx.JSON(w, http.StatusCreated, map[string]any{
		"id":         id,
		"token":      token,
		"expires_at": expires,
		"command":    command,
		"api_url":    apiURL,
		"rdp_user":   body.RdpUser,
		"note":       "Run in elevated PowerShell on the Windows host. Installer enables RDP, creates the local RDP user (random password printed once), and starts the FleetDeck RMM service.",
		"warning":    "Store the enrollment token securely. The RDP password is only shown on the Windows host at install time.",
	})
}
