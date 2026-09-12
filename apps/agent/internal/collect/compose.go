package collect

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var allowedComposeActions = map[string]struct{}{
	"up": {}, "down": {}, "start": {}, "stop": {}, "restart": {}, "pull": {},
}

type composeProjectMeta struct {
	WorkingDir  string
	ConfigFiles []string
	ContainerIDs []string
	ImageRefs   []string
}

func ComposeAction(ctx context.Context, projectName, action string) error {
	action = strings.ToLower(strings.TrimSpace(action))
	projectName = strings.TrimSpace(projectName)
	if projectName == "" {
		return fmt.Errorf("project_name is required")
	}
	if _, ok := allowedComposeActions[action]; !ok {
		return fmt.Errorf("unsupported compose action %q", action)
	}

	meta, err := resolveComposeProject(ctx, projectName)
	if err != nil {
		return err
	}
	if len(meta.ContainerIDs) == 0 && action != "up" && action != "pull" {
		return fmt.Errorf("no containers found for compose project %q", projectName)
	}

	switch action {
	case "start", "stop", "restart":
		return composeLifecycleAPI(ctx, meta.ContainerIDs, action)
	case "down":
		// Stop then remove project containers (no -v; keep volumes).
		if err := composeLifecycleAPI(ctx, meta.ContainerIDs, "stop"); err != nil {
			return err
		}
		for _, id := range meta.ContainerIDs {
			if err := ContainerAction(ctx, id, "remove"); err != nil {
				return fmt.Errorf("remove %s: %w", id[:min(12, len(id))], err)
			}
		}
		return nil
	case "pull":
		if len(meta.ConfigFiles) > 0 {
			if err := runDockerCompose(ctx, projectName, meta, "pull"); err == nil {
				return nil
			}
		}
		return composePullImages(ctx, meta.ImageRefs)
	case "up":
		if len(meta.ConfigFiles) == 0 {
			// Without compose files we can only start existing containers.
			if len(meta.ContainerIDs) == 0 {
				return fmt.Errorf("compose up needs project config_files labels on the host (com.docker.compose.project.config_files)")
			}
			return composeLifecycleAPI(ctx, meta.ContainerIDs, "start")
		}
		return runDockerCompose(ctx, projectName, meta, "up", "-d", "--remove-orphans")
	default:
		return fmt.Errorf("unsupported compose action %q", action)
	}
}

func resolveComposeProject(ctx context.Context, projectName string) (composeProjectMeta, error) {
	var containers []struct {
		ID     string            `json:"Id"`
		Image  string            `json:"Image"`
		Labels map[string]string `json:"Labels"`
	}
	if err := dockerGET(ctx, "/containers/json?all=true", &containers); err != nil {
		return composeProjectMeta{}, err
	}
	meta := composeProjectMeta{}
	seenImages := map[string]struct{}{}
	for _, c := range containers {
		if c.Labels == nil {
			continue
		}
		if c.Labels["com.docker.compose.project"] != projectName {
			continue
		}
		meta.ContainerIDs = append(meta.ContainerIDs, c.ID)
		if img := strings.TrimSpace(c.Image); img != "" {
			if _, ok := seenImages[img]; !ok {
				seenImages[img] = struct{}{}
				meta.ImageRefs = append(meta.ImageRefs, img)
			}
		}
		if meta.WorkingDir == "" {
			meta.WorkingDir = strings.TrimSpace(c.Labels["com.docker.compose.project.working_dir"])
		}
		if len(meta.ConfigFiles) == 0 {
			raw := strings.TrimSpace(c.Labels["com.docker.compose.project.config_files"])
			if raw != "" {
				for _, part := range strings.Split(raw, ",") {
					part = strings.TrimSpace(part)
					if part != "" {
						meta.ConfigFiles = append(meta.ConfigFiles, part)
					}
				}
			}
		}
	}
	return meta, nil
}

func composeLifecycleAPI(ctx context.Context, ids []string, action string) error {
	var firstErr error
	ok := 0
	for _, id := range ids {
		if err := ContainerAction(ctx, id, action); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s %s: %w", action, id[:min(12, len(id))], err)
			}
			continue
		}
		ok++
	}
	if ok == 0 && firstErr != nil {
		return firstErr
	}
	if firstErr != nil {
		return fmt.Errorf("%s partially applied (%d/%d): %w", action, ok, len(ids), firstErr)
	}
	return nil
}

func composePullImages(ctx context.Context, images []string) error {
	if len(images) == 0 {
		return fmt.Errorf("no images to pull for compose project")
	}
	var firstErr error
	ok := 0
	for _, ref := range images {
		if strings.HasPrefix(ref, "sha256:") {
			continue
		}
		q := url.Values{}
		image, tag := splitImageRef(ref)
		q.Set("fromImage", image)
		if tag != "" {
			q.Set("tag", tag)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://docker/images/create?"+q.Encode(), nil)
		if err != nil {
			return err
		}
		res, err := dockerHTTPClient().Do(req)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 8<<20))
		res.Body.Close()
		if res.StatusCode >= 300 {
			if firstErr == nil {
				firstErr = fmt.Errorf("pull %s: HTTP %d", ref, res.StatusCode)
			}
			continue
		}
		ok++
	}
	if ok == 0 && firstErr != nil {
		return firstErr
	}
	return nil
}

func splitImageRef(ref string) (image, tag string) {
	// registry:port/name:tag — split on last ":" only when not a digest.
	if strings.Contains(ref, "@sha256:") {
		return ref, ""
	}
	i := strings.LastIndex(ref, ":")
	if i <= 0 || strings.Contains(ref[i+1:], "/") {
		return ref, "latest"
	}
	return ref[:i], ref[i+1:]
}

func runDockerCompose(ctx context.Context, project string, meta composeProjectMeta, args ...string) error {
	bin, err := exec.LookPath("docker")
	if err != nil {
		return fmt.Errorf("docker CLI not found for compose action")
	}
	cmdArgs := []string{"compose", "-p", project}
	if meta.WorkingDir != "" {
		cmdArgs = append(cmdArgs, "--project-directory", meta.WorkingDir)
	}
	for _, f := range meta.ConfigFiles {
		path := f
		if meta.WorkingDir != "" && !filepath.IsAbs(f) {
			path = filepath.Join(meta.WorkingDir, f)
		}
		cmdArgs = append(cmdArgs, "-f", path)
	}
	cmdArgs = append(cmdArgs, args...)

	cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(cctx, bin, cmdArgs...)
	if meta.WorkingDir != "" {
		cmd.Dir = meta.WorkingDir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		if len(msg) > 2000 {
			msg = msg[:2000] + "…"
		}
		return fmt.Errorf("docker compose %s: %s", args[0], msg)
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
