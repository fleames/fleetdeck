package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fleetdeck/fleetdeck/apps/agent/internal/buffer"
	"github.com/fleetdeck/fleetdeck/apps/agent/internal/collect"
)

const agentVersion = "0.4.6-dev"

const (
	inventoryInterval    = 60 * time.Second
	heartbeatInterval    = 30 * time.Second
	commandLongPollWait  = 25 * time.Second
	commandRetryInterval = 2 * time.Second
)

type enrollResponse struct {
	ServerID     string `json:"server_id"`
	AgentID      string `json:"agent_id"`
	PublicID     string `json:"public_id"`
	Secret       string `json:"secret"`
	APIPublicURL string `json:"api_public_url"`
}

type credentials struct {
	APIURL   string `json:"api_url"`
	PublicID string `json:"public_id"`
	Secret   string `json:"secret"`
	ServerID string `json:"server_id"`
	AgentID  string `json:"agent_id"`
}

func main() {
	apiURL := flag.String("api", getenv("FLEETDECK_URL", "http://localhost:8080"), "FleetDeck API base URL")
	token := flag.String("token", os.Getenv("FLEETDECK_ENROLLMENT_TOKEN"), "Enrollment token")
	stateDir := flag.String("state-dir", getenv("FLEETDECK_STATE_DIR", defaultStateDir()), "Credential state directory")
	interval := flag.Duration("interval", 10*time.Second, "Metrics collection interval")
	enrollOnly := flag.Bool("enroll", false, "Enroll then exit")
	doUninstall := flag.Bool("uninstall", false, "Uninstall agent from this host (root/systemd; Linux)")
	doUpdate := flag.Bool("update", false, "Apply staged agent update (root/systemd; Linux)")
	uninstallDelay := flag.Duration("delay", 0, "Delay before -uninstall/-update (used when scheduled from panel)")
	flag.Parse()

	if *doUninstall {
		if err := runUninstall(*uninstallDelay); err != nil {
			fmt.Fprintf(os.Stderr, "uninstall failed: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if *doUpdate {
		if err := runApplyUpdate(*uninstallDelay); err != nil {
			fmt.Fprintf(os.Stderr, "update failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	credPath := filepath.Join(*stateDir, "credentials.json")
	creds, err := loadCredentials(credPath)
	if err != nil || creds.PublicID == "" {
		if *token == "" {
			fmt.Fprintln(os.Stderr, "No credentials found. Provide -token / FLEETDECK_ENROLLMENT_TOKEN to enroll.")
			os.Exit(1)
		}
		creds, err = enroll(*apiURL, *token, *stateDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "enroll failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Enrolled successfully. Credentials stored in", credPath)
		if *enrollOnly {
			return
		}
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	unlock, err := acquireInstanceLock(*stateDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "instance lock: %v\n", err)
		os.Exit(1)
	}
	defer unlock()

	var netPrev *collect.NetCounter
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	invTicker := time.NewTicker(inventoryInterval)
	defer invTicker.Stop()
	hbTicker := time.NewTicker(heartbeatInterval)
	defer hbTicker.Stop()

	fmt.Printf("FleetDeck agent %s reporting to %s every %s\n", agentVersion, creds.APIURL, interval.String())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	spool := &buffer.Spool{Dir: *stateDir}
	// Inventory first (host identity), then metrics — so a slow Docker stats path cannot block identity updates.
	if err := reportInventory(ctx, creds); err != nil {
		fmt.Fprintf(os.Stderr, "inventory: %v\n", err)
	}
	netPrev = reportOnce(ctx, creds, netPrev, spool)
	reportHeartbeat(ctx, creds, spool)
	go runCommandPoller(ctx, creds)

	for {
		select {
		case <-stop:
			fmt.Println("shutting down")
			cancel()
			return
		case <-ticker.C:
			netPrev = reportOnce(ctx, creds, netPrev, spool)
		case <-invTicker.C:
			if err := reportInventory(ctx, creds); err != nil {
				fmt.Fprintf(os.Stderr, "inventory: %v\n", err)
			}
		case <-hbTicker.C:
			reportHeartbeat(ctx, creds, spool)
		}
	}
}

func reportOnce(ctx context.Context, creds credentials, netPrev *collect.NetCounter, spool *buffer.Spool) *collect.NetCounter {
	sample, next, err := collect.SampleHost(ctx, netPrev)
	if err != nil {
		fmt.Fprintf(os.Stderr, "host sample: %v\n", err)
		return netPrev
	}
	containerSamples, err := collect.SampleContainerStats(ctx)
	if err != nil {
		containerSamples = nil
	}
	payload := map[string]any{
		"schema_version": 1,
		"agent_version":  agentVersion,
		"sent_at":        time.Now().UTC(),
		"host":           []collect.HostSample{sample},
		"containers":     containerSamples,
	}
	metricsURL := creds.APIURL + "/agent/v1/metrics"
	bearer := creds.PublicID + ":" + creds.Secret
	if err := postJSON(metricsURL, bearer, payload, nil); err != nil {
		fmt.Fprintf(os.Stderr, "metrics: %v (spooling)\n", err)
		if spool != nil {
			if serr := spool.Append(payload); serr != nil {
				fmt.Fprintf(os.Stderr, "metrics spool: %v\n", serr)
			}
		}
	} else if spool != nil {
		if ferr := spool.FlushAll(func(raw json.RawMessage) error {
			var buffered map[string]any
			if err := json.Unmarshal(raw, &buffered); err != nil {
				return err
			}
			return postJSON(metricsURL, bearer, buffered, nil)
		}); ferr != nil {
			fmt.Fprintf(os.Stderr, "metrics flush: %v\n", ferr)
		}
	}
	return next
}

func reportHeartbeat(ctx context.Context, creds credentials, spool *buffer.Spool) {
	available, healthy := collect.ProbeDocker(ctx)
	bearer := creds.PublicID + ":" + creds.Secret
	hb := map[string]any{
		"agent_version":  agentVersion,
		"docker_healthy": !available || healthy,
	}
	if spool != nil {
		n, age := spool.Stats()
		hb["buffered_samples"] = n
		hb["oldest_buffer_age_sec"] = age
	}
	if err := postJSON(creds.APIURL+"/agent/v1/heartbeat", bearer, hb, nil); err != nil {
		fmt.Fprintf(os.Stderr, "heartbeat: %v\n", err)
	}
}

func reportInventory(ctx context.Context, creds credentials) error {
	host, err := collect.CollectHostInfo(ctx)
	if err != nil {
		return err
	}
	docker := collect.CollectDocker(ctx)
	payload := map[string]any{
		"schema_version": 1,
		"agent_version":  agentVersion,
		"host":           host,
		"docker": map[string]any{
			"available":      docker.Available,
			"version":        docker.Version,
			"api_version":    docker.APIVersion,
			"daemon_healthy": docker.DaemonHealthy,
		},
		"containers":       docker.Containers,
		"images":           docker.Images,
		"volumes":          docker.Volumes,
		"networks":         docker.Networks,
		"compose_projects": docker.ComposeProjects,
	}
	return postJSON(creds.APIURL+"/agent/v1/inventory", creds.PublicID+":"+creds.Secret, payload, nil)
}

func runCommandPoller(ctx context.Context, creds credentials) {
	for ctx.Err() == nil {
		started := time.Now()
		err := pollCommands(ctx, creds)
		if ctx.Err() != nil {
			return
		}
		// Avoid a tight loop against an older API that ignores the long-poll wait,
		// and back off transient failures without adding a separate retry storm.
		delay := commandRetryInterval - time.Since(started)
		if err != nil {
			delay = commandRetryInterval
		}
		if delay <= 0 {
			continue
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func pollCommands(ctx context.Context, creds credentials) error {
	var resp struct {
		Data []struct {
			ID      string          `json:"id"`
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		} `json:"data"`
	}
	url := fmt.Sprintf("%s/agent/v1/commands?wait=%s", creds.APIURL, commandLongPollWait)
	if err := getJSON(ctx, url, creds.PublicID+":"+creds.Secret, &resp); err != nil {
		return err
	}
	for _, cmd := range resp.Data {
		ok := true
		result := ""
		errText := ""
		requestUninstall := false
		requestUpdate := false
		switch cmd.Type {
		case "container.logs":
			var p struct {
				ContainerID string `json:"container_id"`
				Tail        int    `json:"tail"`
				Since       int64  `json:"since"`
			}
			_ = json.Unmarshal(cmd.Payload, &p)
			text, err := collect.FetchContainerLogs(ctx, p.ContainerID, p.Tail, p.Since)
			if err != nil {
				ok = false
				errText = err.Error()
			} else {
				result = text
			}
		case "compose.up", "compose.down", "compose.start", "compose.stop", "compose.restart", "compose.pull":
			var p struct {
				ProjectName string `json:"project_name"`
				Action      string `json:"action"`
			}
			_ = json.Unmarshal(cmd.Payload, &p)
			action := p.Action
			if action == "" {
				action = strings.TrimPrefix(cmd.Type, "compose.")
			}
			if err := collect.ComposeAction(ctx, p.ProjectName, action); err != nil {
				ok = false
				errText = err.Error()
			} else {
				result = "ok"
			}
		case "container.start", "container.stop", "container.restart", "container.pause", "container.unpause", "container.remove":
			var p struct {
				ContainerID string `json:"container_id"`
				Action      string `json:"action"`
			}
			_ = json.Unmarshal(cmd.Payload, &p)
			action := p.Action
			if action == "" {
				action = strings.TrimPrefix(cmd.Type, "container.")
			}
			if err := collect.ContainerAction(ctx, p.ContainerID, action); err != nil {
				ok = false
				errText = err.Error()
			} else {
				result = "ok"
			}
		case "container.inspect":
			var p struct {
				ContainerID string `json:"container_id"`
			}
			_ = json.Unmarshal(cmd.Payload, &p)
			text, err := collect.InspectContainerEnv(ctx, p.ContainerID)
			if err != nil {
				ok = false
				errText = err.Error()
			} else {
				result = text
			}
		case "agent.uninstall", "host.uninstall":
			if err := validateUninstallReady(); err != nil {
				ok = false
				errText = err.Error()
			} else {
				result = "uninstall requested"
				requestUninstall = true
			}
		case "agent.update":
			var p struct {
				CDNBase string `json:"cdn_base"`
				Channel string `json:"channel"`
				URL     string `json:"url"`
			}
			_ = json.Unmarshal(cmd.Payload, &p)
			if err := validateUpdateReady(); err != nil {
				ok = false
				errText = err.Error()
			} else {
				cdnBase := p.CDNBase
				channel := p.Channel
				if p.URL != "" && cdnBase == "" {
					// Allow full channel URL like https://cdn…/fleetdeck/latest
					cdnBase = strings.TrimRight(p.URL, "/")
					if i := strings.LastIndex(cdnBase, "/"); i > 0 {
						channel = cdnBase[i+1:]
						cdnBase = cdnBase[:i]
					}
				}
				binURL, err := stageUpdateFromCDN(cdnBase, channel)
				if err != nil {
					ok = false
					errText = err.Error()
				} else {
					result = "update staged from " + binURL + "; restarting"
					requestUpdate = true
				}
			}
		default:
			ok = false
			errText = "unsupported command type"
		}
		_ = postJSON(creds.APIURL+"/agent/v1/commands/"+cmd.ID+"/result", creds.PublicID+":"+creds.Secret, map[string]any{
			"ok":     ok,
			"result": result,
			"error":  errText,
		}, nil)
		// Trigger AFTER result POST so the API can observe completion before systemd stops/restarts us.
		if requestUninstall {
			if err := triggerUninstall(); err != nil {
				fmt.Fprintf(os.Stderr, "uninstall trigger: %v\n", err)
			}
		}
		if requestUpdate {
			if err := triggerUpdate(); err != nil {
				fmt.Fprintf(os.Stderr, "update trigger: %v\n", err)
			}
		}
	}
	return nil
}

func enroll(apiURL, token, stateDir string) (credentials, error) {
	host, _ := collect.CollectHostInfo(context.Background())
	body := map[string]string{
		"token":         token,
		"agent_version": agentVersion,
		"hostname":      host.Hostname,
		"os":            host.OSName,
		"arch":          host.Arch,
	}
	var resp enrollResponse
	if err := postJSON(apiURL+"/agent/v1/enroll", "", body, &resp); err != nil {
		return credentials{}, err
	}
	creds := credentials{
		APIURL:   firstNonEmpty(resp.APIPublicURL, apiURL),
		PublicID: resp.PublicID,
		Secret:   resp.Secret,
		ServerID: resp.ServerID,
		AgentID:  resp.AgentID,
	}
	if err := writeCredentials(stateDir, creds); err != nil {
		return credentials{}, err
	}
	return creds, nil
}

// writeCredentials persists agent credentials with restrictive permissions (dir 0700, file 0600).
// Chmod after write defeats umask so mode stays 0600 even if the process umask is permissive.
func writeCredentials(stateDir string, creds credentials) error {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}
	_ = os.Chmod(stateDir, 0o700)
	b, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(stateDir, "credentials.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func postJSON(url, bearer string, payload any, out any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", res.StatusCode, string(body))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
}

func getJSON(ctx context.Context, url, bearer string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	client := &http.Client{Timeout: commandLongPollWait + 10*time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", res.StatusCode, string(body))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
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

func defaultStateDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".fleetdeck"
	}
	return filepath.Join(home, ".fleetdeck")
}

func getenv(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
