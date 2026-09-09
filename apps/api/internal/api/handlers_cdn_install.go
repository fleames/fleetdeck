package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/fleetdeck/fleetdeck/apps/api/internal/httpx"
)

// handleInstallCommand returns a short CDN one-liner (API via Cloudflare Tunnel / API_PUBLIC_URL).
func (s *Server) handleInstallCommand(w http.ResponseWriter, r *http.Request) {
	body, err := s.parseInstallRequest(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation", err.Error())
		return
	}
	cdn := strings.TrimRight(s.cfg.AgentCDNBase, "/")
	if cdn == "" {
		cdn = "https://cdn.tarkovbot.com/fleetdeck"
	}
	channel := s.cfg.AgentCDNChannel
	if channel == "" {
		channel = "latest"
	}

	apiURL := body.APIURL
	if apiURL == "" {
		apiURL = strings.TrimRight(s.cfg.APIPublicURL, "/")
	}
	escToken := strings.ReplaceAll(body.Token, `'`, `'\''`)

	// Token-only when API URL matches the configured public URL (tunnel hostname).
	command := fmt.Sprintf(
		`curl -fsSL %s/install.sh | sudo bash -s -- --token '%s'`,
		cdn, escToken,
	)
	if apiURL != "" && !strings.EqualFold(apiURL, strings.TrimRight(s.cfg.APIPublicURL, "/")) {
		escAPI := strings.ReplaceAll(apiURL, `'`, `'\''`)
		command = fmt.Sprintf(
			`curl -fsSL %s/install.sh | sudo bash -s -- --token '%s' --api '%s'`,
			cdn, escToken, escAPI,
		)
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"command":     command,
		"cdn_base":    cdn,
		"cdn_channel": channel,
		"api_url":     apiURL,
		"tunnel":      true,
		"note":        "VPS downloads from CDN and dials API_PUBLIC_URL (Cloudflare Tunnel). Keep FleetDeck + tunnel online.",
	})
}
