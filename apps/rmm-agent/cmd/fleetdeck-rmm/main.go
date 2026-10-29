package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

const agentVersion = "0.1.4-dev"

const (
	heartbeatInterval   = 60 * time.Second
	commandLongPollWait = 30 * time.Second
)

type enrollResponse struct {
	HostID       string `json:"host_id"`
	PublicID     string `json:"public_id"`
	Secret       string `json:"secret"`
	APIPublicURL string `json:"api_public_url"`
}

type credentials struct {
	APIURL   string `json:"api_url"`
	PublicID string `json:"public_id"`
	Secret   string `json:"secret"`
	HostID   string `json:"host_id"`
}

func main() {
	apiURL := flag.String("api", getenv("FLEETDECK_URL", "http://localhost:8080"), "FleetDeck API base URL")
	token := flag.String("token", os.Getenv("FLEETDECK_RMM_ENROLLMENT_TOKEN"), "RMM enrollment token")
	stateDir := flag.String("state-dir", getenv("FLEETDECK_RMM_STATE_DIR", defaultStateDir()), "Credential state directory")
	enrollOnly := flag.Bool("enroll", false, "Enroll then exit")
	doService := flag.Bool("service", false, "Run as Windows service (install via install-rmm.ps1)")
	rdpUser := flag.String("rdp-user", os.Getenv("FLEETDECK_RDP_USER"), "Local RDP username to register with panel")
	rdpPassword := flag.String("rdp-password", os.Getenv("FLEETDECK_RDP_PASSWORD"), "Local RDP password to register with panel")
	connectGateway := flag.String("connect", "", "Native connect: gateway host:port")
	connectToken := flag.String("connect-token", "", "Native connect AUTH token")
	connectLocal := flag.Int("connect-local-port", 13389, "Local port for mstsc forward")
	flag.Parse()

	if *connectGateway != "" {
		if err := runNativeConnect(*connectGateway, *connectToken, *connectLocal); err != nil {
			fmt.Fprintf(os.Stderr, "connect failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if *doService && runtime.GOOS == "windows" {
		if err := runWindowsService(*apiURL, *token, *stateDir); err != nil {
			fmt.Fprintf(os.Stderr, "service: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := runAgent(context.Background(), *apiURL, *token, *stateDir, *enrollOnly, *rdpUser, *rdpPassword); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func runAgent(ctx context.Context, apiURL, token, stateDir string, enrollOnly bool, rdpUser, rdpPassword string) error {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}
	if rdpUser == "" || rdpPassword == "" {
		u, p, ok := loadRDPCredFile(filepath.Join(stateDir, "rdp-credentials.txt"))
		if ok {
			if rdpUser == "" {
				rdpUser = u
			}
			if rdpPassword == "" {
				rdpPassword = p
			}
		}
	}
	// Clear "must change password at next logon" so Guacamole does not report
	// "Credentials expired." for an otherwise-valid local account.
	if rdpUser != "" {
		ensureLocalRDPAccountUsable(rdpUser)
	}
	credPath := filepath.Join(stateDir, "credentials.json")
	creds, err := loadCredentials(credPath)
	if err != nil || creds.PublicID == "" {
		if token == "" {
			return fmt.Errorf("no credentials found; provide -token / FLEETDECK_RMM_ENROLLMENT_TOKEN")
		}
		creds, err = enroll(apiURL, token, stateDir, rdpUser, rdpPassword)
		if err != nil {
			return fmt.Errorf("enroll failed: %w", err)
		}
		fmt.Println("Enrolled successfully. Credentials stored in", credPath)
		if enrollOnly {
			return nil
		}
	} else {
		// Credentials on disk may be orphaned if the host was removed from the panel.
		if err := verifyCredentials(creds); err != nil {
			fmt.Fprintf(os.Stderr, "Stored credentials rejected by API (%v). Re-enrolling…\n", err)
			_ = os.Remove(credPath)
			if token == "" {
				return fmt.Errorf("host is no longer registered in FleetDeck; re-run install with a fresh enrollment token")
			}
			creds, err = enroll(apiURL, token, stateDir, rdpUser, rdpPassword)
			if err != nil {
				return fmt.Errorf("re-enroll failed: %w", err)
			}
			fmt.Println("Re-enrolled successfully. Credentials stored in", credPath)
			if enrollOnly {
				return nil
			}
		} else if enrollOnly {
			if rdpUser != "" && rdpPassword != "" {
				if err := pushRDPCredentials(creds, rdpUser, rdpPassword); err != nil {
					fmt.Fprintf(os.Stderr, "warning: could not sync RDP password to panel: %v\n", err)
				} else {
					fmt.Println("RDP credentials synced to FleetDeck.")
				}
			}
			fmt.Println("Already enrolled:", credPath)
			return nil
		}
	}

	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	var sessions sync.Map // session_id -> cancel
	go heartbeatLoop(ctx, creds, stateDir)
	go commandLoop(ctx, creds, stateDir, &sessions)

	<-ctx.Done()
	sessions.Range(func(_, v any) bool {
		if c, ok := v.(context.CancelFunc); ok {
			c()
		}
		return true
	})
	return nil
}

func defaultStateDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("ProgramData"), "FleetDeck", "RMM")
	}
	return filepath.Join(os.TempDir(), "fleetdeck-rmm")
}

func getenv(k, d string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return d
}

func loadCredentials(path string) (credentials, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return credentials{}, err
	}
	var c credentials
	if err := json.Unmarshal(b, &c); err != nil {
		return credentials{}, err
	}
	return c, nil
}

func saveCredentials(dir string, c credentials) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "credentials.json"), b, 0o600)
}

func enroll(apiURL, token, stateDir, rdpUser, rdpPassword string) (credentials, error) {
	hostname, _ := os.Hostname()
	rdpEnabled, rdpPort := detectRDP()
	sku := detectWindowsSKU()
	body := map[string]any{
		"token":                    token,
		"agent_version":            agentVersion,
		"hostname":                 hostname,
		"os_name":                  runtime.GOOS,
		"os_version":               osVersion(),
		"arch":                     runtime.GOARCH,
		"rdp_enabled":              rdpEnabled,
		"rdp_port":                 rdpPort,
		"name":                     hostname,
		"os_edition":               sku.Edition,
		"os_product_type":          sku.ProductType,
		"rdp_concurrent_sessions":  sku.ConcurrentSessions,
	}
	if rdpUser != "" {
		body["rdp_username"] = rdpUser
	}
	if rdpPassword != "" {
		body["rdp_password"] = rdpPassword
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(apiURL, "/")+"/rmm/v1/enroll", bytes.NewReader(raw))
	if err != nil {
		return credentials{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return credentials{}, err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return credentials{}, fmt.Errorf("enroll HTTP %d: %s", res.StatusCode, string(b))
	}
	var er enrollResponse
	if err := json.Unmarshal(b, &er); err != nil {
		return credentials{}, err
	}
	api := apiURL
	if er.APIPublicURL != "" {
		api = er.APIPublicURL
	}
	c := credentials{APIURL: api, PublicID: er.PublicID, Secret: er.Secret, HostID: er.HostID}
	if err := saveCredentials(stateDir, c); err != nil {
		return credentials{}, err
	}
	return c, nil
}

func authHeader(c credentials) string {
	return "Bearer " + c.PublicID + ":" + c.Secret
}

func heartbeatLoop(ctx context.Context, c credentials, stateDir string) {
	t := time.NewTicker(heartbeatInterval)
	defer t.Stop()
	send := func() {
		hostname, _ := os.Hostname()
		rdpEnabled, rdpPort := detectRDP()
		sku := detectWindowsSKU()
		body := map[string]any{
			"agent_version":           agentVersion,
			"hostname":                hostname,
			"os_version":              osVersion(),
			"rdp_enabled":             rdpEnabled,
			"rdp_port":                rdpPort,
			"os_edition":              sku.Edition,
			"os_product_type":         sku.ProductType,
			"rdp_concurrent_sessions": sku.ConcurrentSessions,
		}
		// Keep panel secret in sync with the local install file (source of truth for the Windows account).
		if u, p, ok := loadRDPCredFile(filepath.Join(stateDir, "rdp-credentials.txt")); ok {
			body["rdp_username"] = u
			body["rdp_password"] = p
		}
		raw, _ := json.Marshal(body)
		req, err := http.NewRequest(http.MethodPost, strings.TrimRight(c.APIURL, "/")+"/rmm/v1/heartbeat", bytes.NewReader(raw))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authHeader(c))
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return
		}
		res.Body.Close()
	}
	send()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			send()
		}
	}
}

func commandLoop(ctx context.Context, c credentials, stateDir string, sessions *sync.Map) {
	for {
		if ctx.Err() != nil {
			return
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			strings.TrimRight(c.APIURL, "/")+"/rmm/v1/commands?wait="+commandLongPollWait.String(), nil)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		req.Header.Set("Authorization", authHeader(c))
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		var payload struct {
			Data []struct {
				ID      string          `json:"id"`
				Type    string          `json:"type"`
				Payload json.RawMessage `json:"payload"`
			} `json:"data"`
		}
		_ = json.NewDecoder(res.Body).Decode(&payload)
		res.Body.Close()
		for _, cmd := range payload.Data {
			ok, result, errMsg, applyUpdate := handleCommand(ctx, c, stateDir, cmd.Type, cmd.Payload, sessions)
			reportCommand(c, cmd.ID, ok, result, errMsg)
			// Apply AFTER result POST so the panel wait can observe completion before service restart.
			if applyUpdate {
				if err := triggerRMMUpdate(stateDir); err != nil {
					fmt.Fprintf(os.Stderr, "update trigger: %v\n", err)
				}
			}
		}
	}
}

func handleCommand(ctx context.Context, c credentials, stateDir, typ string, payload json.RawMessage, sessions *sync.Map) (ok bool, result, errMsg string, applyUpdate bool) {
	switch typ {
	case "session.create":
		var p struct {
			SessionID   string `json:"session_id"`
			TunnelURL   string `json:"tunnel_url"`
			TunnelToken string `json:"tunnel_token"`
			RDPHost     string `json:"rdp_host"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return false, "", err.Error(), false
		}
		if p.RDPHost == "" {
			p.RDPHost = "127.0.0.1"
		}
		rdpEnabled, rdpPort := detectRDP()
		if !rdpEnabled {
			return false, "", "RDP is not enabled or not listening on this host", false
		}
		// Quick probe
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(p.RDPHost, fmt.Sprintf("%d", rdpPort)), 3*time.Second)
		if err != nil {
			return false, "", fmt.Sprintf("cannot reach local RDP %s:%d: %v", p.RDPHost, rdpPort, err), false
		}
		_ = conn.Close()

		sctx, cancel := context.WithCancel(ctx)
		if old, ok := sessions.LoadOrStore(p.SessionID, cancel); ok {
			cancel()
			_ = old
			return false, "", "session already active locally", false
		}
		go func() {
			defer sessions.Delete(p.SessionID)
			defer cancel()
			if err := runSessionTunnel(sctx, c, p.SessionID, p.TunnelURL, p.TunnelToken, p.RDPHost, rdpPort); err != nil {
				fmt.Fprintf(os.Stderr, "session tunnel: %v\n", err)
			}
		}()
		return true, "tunnel starting", "", false
	case "session.close":
		var p struct {
			SessionID string `json:"session_id"`
		}
		_ = json.Unmarshal(payload, &p)
		if v, ok := sessions.Load(p.SessionID); ok {
			if cancel, ok := v.(context.CancelFunc); ok {
				cancel()
			}
			sessions.Delete(p.SessionID)
		}
		return true, "closed", "", false
	case "agent.update":
		if runtime.GOOS != "windows" {
			return false, "", "RMM agent update is only supported on Windows", false
		}
		var p struct {
			CDNBase string `json:"cdn_base"`
			Channel string `json:"channel"`
			URL     string `json:"url"`
		}
		_ = json.Unmarshal(payload, &p)
		cdnBase := p.CDNBase
		channel := p.Channel
		if p.URL != "" && cdnBase == "" {
			cdnBase = strings.TrimRight(p.URL, "/")
			if i := strings.LastIndex(cdnBase, "/"); i > 0 {
				channel = cdnBase[i+1:]
				cdnBase = cdnBase[:i]
			}
		}
		binURL, err := stageRMMUpdateFromCDN(cdnBase, channel, stateDir)
		if err != nil {
			return false, "", err.Error(), false
		}
		return true, "update staged from " + binURL + "; restarting", "", true
	default:
		return false, "", "unknown command", false
	}
}

func reportCommand(c credentials, id string, ok bool, result, errMsg string) {
	body, _ := json.Marshal(map[string]any{"ok": ok, "result": result, "error": errMsg})
	req, err := http.NewRequest(http.MethodPost,
		strings.TrimRight(c.APIURL, "/")+"/rmm/v1/commands/"+id+"/result", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader(c))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	res.Body.Close()
}

func runSessionTunnel(ctx context.Context, c credentials, sessionID, tunnelURL, token, rdpHost string, rdpPort int) error {
	u, err := url.Parse(tunnelURL)
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("session_id", sessionID)
	q.Set("token", token)
	u.RawQuery = q.Encode()
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	}

	hdr := http.Header{}
	hdr.Set("Authorization", authHeader(c))
	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	ws, _, err := dialer.DialContext(ctx, u.String(), hdr)
	if err != nil {
		return err
	}
	defer ws.Close()

	var (
		mu       sync.Mutex
		rdpConn  net.Conn
		writeMu  sync.Mutex
	)
	closeRDP := func() {
		mu.Lock()
		defer mu.Unlock()
		if rdpConn != nil {
			_ = rdpConn.Close()
			rdpConn = nil
		}
	}
	defer closeRDP()

	go func() {
		<-ctx.Done()
		_ = ws.Close()
		closeRDP()
	}()

	for {
		mt, data, err := ws.ReadMessage()
		if err != nil {
			return err
		}
		if mt == websocket.TextMessage {
			var msg struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(data, &msg) == nil {
				switch msg.Type {
				case "open_rdp":
					closeRDP()
					conn, err := net.DialTimeout("tcp", net.JoinHostPort(rdpHost, fmt.Sprintf("%d", rdpPort)), 5*time.Second)
					if err != nil {
						continue
					}
					mu.Lock()
					rdpConn = conn
					mu.Unlock()
					go func(conn net.Conn) {
						buf := make([]byte, 32*1024)
						for {
							n, err := conn.Read(buf)
							if n > 0 {
								writeMu.Lock()
								werr := ws.WriteMessage(websocket.BinaryMessage, buf[:n])
								writeMu.Unlock()
								if werr != nil {
									return
								}
							}
							if err != nil {
								return
							}
						}
					}(conn)
				case "close_rdp":
					closeRDP()
				}
			}
			continue
		}
		if mt == websocket.BinaryMessage {
			mu.Lock()
			conn := rdpConn
			mu.Unlock()
			if conn != nil {
				_, _ = conn.Write(data)
			}
		}
	}
}

func verifyCredentials(c credentials) error {
	body, _ := json.Marshal(map[string]any{"agent_version": agentVersion})
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(c.APIURL, "/")+"/rmm/v1/heartbeat", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader(c))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	if res.StatusCode >= 300 {
		b, _ := io.ReadAll(res.Body)
		return fmt.Errorf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

func pushRDPCredentials(c credentials, user, pass string) error {
	body, _ := json.Marshal(map[string]any{
		"rdp_username": user,
		"rdp_password": pass,
		"rdp_enabled":  true,
	})
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(c.APIURL, "/")+"/rmm/v1/heartbeat", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader(c))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	res.Body.Close()
	return nil
}

func loadRDPCredFile(path string) (user, pass string, ok bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", "", false
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "username=") {
			user = strings.TrimPrefix(line, "username=")
		}
		if strings.HasPrefix(line, "password=") {
			pass = strings.TrimPrefix(line, "password=")
		}
	}
	if user == "" || pass == "" {
		return "", "", false
	}
	return user, pass, true
}

func runNativeConnect(gateway, token string, localPort int) error {
	if token == "" {
		return fmt.Errorf("-connect-token required")
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", localPort))
	if err != nil {
		return err
	}
	defer ln.Close()

	remote, err := net.DialTimeout("tcp", gateway, 10*time.Second)
	if err != nil {
		return err
	}
	defer remote.Close()
	if _, err := io.WriteString(remote, "AUTH "+token+"\n"); err != nil {
		return err
	}
	buf := make([]byte, 64)
	n, err := remote.Read(buf)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(strings.TrimSpace(string(buf[:n])), "OK") {
		return fmt.Errorf("gateway rejected token: %s", string(buf[:n]))
	}

	fmt.Printf("Authenticated. Open Remote Desktop to 127.0.0.1:%d (waiting for connection)…\n", localPort)
	local, err := ln.Accept()
	if err != nil {
		return err
	}
	defer local.Close()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = io.Copy(remote, local); _ = remote.Close() }()
	go func() { defer wg.Done(); _, _ = io.Copy(local, remote); _ = local.Close() }()
	wg.Wait()
	return nil
}
