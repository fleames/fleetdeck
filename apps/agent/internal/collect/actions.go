package collect

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ContainerAction runs a Docker Engine lifecycle action against a container.
func ContainerAction(ctx context.Context, containerID, action string) error {
	action = strings.ToLower(action)
	path := ""
	switch action {
	case "start":
		path = "/containers/" + containerID + "/start"
	case "stop":
		path = "/containers/" + containerID + "/stop?t=10"
	case "restart":
		path = "/containers/" + containerID + "/restart?t=10"
	case "pause":
		path = "/containers/" + containerID + "/pause"
	case "unpause":
		path = "/containers/" + containerID + "/unpause"
	default:
		return fmt.Errorf("unsupported action %q", action)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://docker"+path, nil)
	if err != nil {
		return err
	}
	res, err := dockerHTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 64_000))
	// 204 No Content and 304 Not Modified are success for Docker lifecycle.
	if res.StatusCode >= 300 && res.StatusCode != 304 {
		return fmt.Errorf("docker %s HTTP %d: %s", action, res.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// InspectContainerEnv returns the Env slice from Docker inspect as JSON {"Env":[...]}.
func InspectContainerEnv(ctx context.Context, containerID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/containers/"+containerID+"/json", nil)
	if err != nil {
		return "", err
	}
	res, err := dockerHTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 4_000_000))
	if err != nil {
		return "", err
	}
	if res.StatusCode >= 300 {
		return "", fmt.Errorf("docker inspect HTTP %d: %s", res.StatusCode, string(body))
	}
	var raw struct {
		Config struct {
			Env []string `json:"Env"`
		} `json:"Config"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", err
	}
	out, err := json.Marshal(map[string]any{"Env": raw.Config.Env})
	if err != nil {
		return "", err
	}
	return string(out), nil
}
