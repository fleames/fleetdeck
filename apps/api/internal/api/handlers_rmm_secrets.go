package api

import (
	"context"
	"encoding/json"
	"log"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Short-lived operator overrides for browser Guacamole (never put passwords in WS query strings).
type rmmSessionCredOverride struct {
	Username string
	Password string
	Expires  time.Time
}

var (
	rmmCredOverrideMu sync.Mutex
	rmmCredOverrides  = map[uuid.UUID]rmmSessionCredOverride{}
)

var (
	cdnChannelRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)
	rdpLocalUserRe = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,20}$`)
)

func validCDNChannel(channel string) bool {
	return cdnChannelRe.MatchString(channel)
}

func validLocalRDPUser(username string) bool {
	return rdpLocalUserRe.MatchString(username)
}

func (s *Server) setRMMSessionCredOverride(sessionID uuid.UUID, username, password string) {
	rmmCredOverrideMu.Lock()
	defer rmmCredOverrideMu.Unlock()
	rmmCredOverrides[sessionID] = rmmSessionCredOverride{
		Username: strings.TrimSpace(username),
		Password: password,
		Expires:  time.Now().UTC().Add(10 * time.Minute),
	}
}

func (s *Server) takeRMMSessionCredOverride(sessionID uuid.UUID) (rmmRDPSecret, bool) {
	rmmCredOverrideMu.Lock()
	defer rmmCredOverrideMu.Unlock()
	o, ok := rmmCredOverrides[sessionID]
	if !ok {
		return rmmRDPSecret{}, false
	}
	if time.Now().UTC().After(o.Expires) {
		delete(rmmCredOverrides, sessionID)
		return rmmRDPSecret{}, false
	}
	// Keep until session ends / expiry so Guacamole reconnects during the same page load work.
	return rmmRDPSecret{Username: o.Username, Password: o.Password}, o.Username != "" && o.Password != ""
}

func (s *Server) clearRMMSessionCredOverride(sessionID uuid.UUID) {
	rmmCredOverrideMu.Lock()
	defer rmmCredOverrideMu.Unlock()
	delete(rmmCredOverrides, sessionID)
}

type rmmRDPSecret struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// formatLocalRDPUsername ensures FreeRDP/NLA can resolve a local Windows account.
// Bare "fleetdeck" often fails NLA; ".\fleetdeck" or "HOST\fleetdeck" succeeds.
func formatLocalRDPUsername(username string) string {
	username = strings.TrimSpace(username)
	if username == "" {
		return username
	}
	if strings.ContainsAny(username, `/\`) {
		return username
	}
	return `.\` + username
}

// localRDPAccountParts splits a local Windows account into Guacamole username + domain.
// Prefer domain="." with a bare username over embedding ".\" in the username field.
func localRDPAccountParts(username string) (user, domain string) {
	username = strings.TrimSpace(username)
	if username == "" {
		return "", ""
	}
	if strings.Contains(username, "/") {
		parts := strings.SplitN(username, "/", 2)
		return parts[1], parts[0]
	}
	if i := strings.LastIndex(username, `\`); i >= 0 {
		dom := username[:i]
		user = username[i+1:]
		if dom == "" || dom == "." {
			return user, "."
		}
		return user, dom
	}
	return username, "."
}

// applyGuacRDPAccount sets username/domain/password for guacd local-account NLA.
func applyGuacRDPAccount(q url.Values, username, password string) {
	user, domain := localRDPAccountParts(username)
	if user != "" {
		q.Set("username", user)
	}
	if domain != "" {
		q.Set("domain", domain)
	}
	if password != "" {
		q.Set("password", password)
	}
}

// sanitizeGuacDisplayParams keeps only safe display/connection knobs from the browser.
// Hostname, port, and credentials are always set server-side (SSRF / credential leak defense).
func sanitizeGuacDisplayParams(q url.Values) url.Values {
	out := url.Values{}
	for _, k := range []string{
		"width", "height", "dpi", "color-depth", "security", "ignore-cert",
		"enable-wallpaper", "resize-method", "disable-audio",
	} {
		if v := strings.TrimSpace(q.Get(k)); v != "" {
			out.Set(k, v)
		}
	}
	out.Set("enable-drive", "false")
	out.Set("create-drive-path", "false")
	return out
}

// applyRMMRDPCredentials overlays stored or session-override RDP credentials onto guacd params.
// Client query credentials are never trusted (passwords must not appear in WebSocket URLs).
// Caller must set hostname/port from the hub tunnel after sanitizeGuacDisplayParams.
func (s *Server) applyRMMRDPCredentials(ctx context.Context, hostID, sessionID uuid.UUID, q url.Values) {
	// Drop auth-related fields; hostname/port are owned by the caller (tunnel listen addr).
	for _, k := range []string{
		"username", "password", "domain",
		"override_credentials", "gateway-hostname", "gateway-port",
		"gateway-username", "gateway-password", "gateway-domain",
		"recording-path", "recording-name", "create-recording-path",
		"enable-sftp", "sftp-hostname", "sftp-port", "sftp-username", "sftp-password",
	} {
		q.Del(k)
	}

	if cred, ok := s.takeRMMSessionCredOverride(sessionID); ok {
		applyGuacRDPAccount(q, cred.Username, cred.Password)
	} else if cred, ok := s.loadRMMRDPSecret(ctx, hostID); ok {
		applyGuacRDPAccount(q, cred.Username, cred.Password)
	}

	// Stable defaults for Windows RDP via guacd (browser may omit some).
	if q.Get("security") == "" {
		q.Set("security", "any")
	}
	if q.Get("ignore-cert") == "" {
		q.Set("ignore-cert", "true")
	}
	if q.Get("width") == "" {
		q.Set("width", "1280")
	}
	if q.Get("height") == "" {
		q.Set("height", "800")
	}
	if q.Get("dpi") == "" {
		q.Set("dpi", "96")
	}
	if q.Get("color-depth") == "" {
		q.Set("color-depth", "32")
	}
	q.Set("enable-drive", "false")
}

func (s *Server) storeRMMRDPSecret(ctx context.Context, hostID uuid.UUID, username, password string) error {
	if s.secrets == nil {
		log.Printf("rmm rdp secret: secrets store unavailable; credentials not persisted for host %s", hostID)
		return nil
	}
	username = strings.TrimSpace(username)
	username = strings.TrimPrefix(username, `.\`)
	if i := strings.LastIndexAny(username, `\`); i >= 0 {
		username = username[i+1:]
	}
	raw, err := json.Marshal(rmmRDPSecret{Username: username, Password: password})
	if err != nil {
		return err
	}
	_, err = s.secrets.Put(ctx, "rmm_rdp", hostID.String(), raw)
	return err
}

func (s *Server) loadRMMRDPSecret(ctx context.Context, hostID uuid.UUID) (rmmRDPSecret, bool) {
	if s.secrets == nil {
		return rmmRDPSecret{}, false
	}
	raw, err := s.secrets.Get(ctx, "rmm_rdp", hostID.String())
	if err != nil {
		return rmmRDPSecret{}, false
	}
	var out rmmRDPSecret
	if err := json.Unmarshal(raw, &out); err != nil || out.Username == "" || out.Password == "" {
		return rmmRDPSecret{}, false
	}
	return out, true
}

func (s *Server) hasRMMRDPSecret(ctx context.Context, hostID uuid.UUID) bool {
	if s.secrets == nil {
		return false
	}
	ok, err := s.secrets.Exists(ctx, "rmm_rdp", hostID.String())
	return err == nil && ok
}
