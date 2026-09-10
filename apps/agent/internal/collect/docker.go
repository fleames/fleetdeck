package collect

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type DockerInventory struct {
	Available       bool             `json:"available"`
	Version         string           `json:"version,omitempty"`
	APIVersion      string           `json:"api_version,omitempty"`
	DaemonHealthy   bool             `json:"daemon_healthy"`
	Containers      []ContainerInfo  `json:"containers,omitempty"`
	Images          []ImageInfo      `json:"images,omitempty"`
	Volumes         []VolumeInfo     `json:"volumes,omitempty"`
	Networks        []NetworkInfo    `json:"networks,omitempty"`
	ComposeProjects []ComposeProject `json:"compose_projects,omitempty"`
	Error           string           `json:"error,omitempty"`
}

type ContainerInfo struct {
	ContainerID    string          `json:"container_id"`
	Name           string          `json:"name"`
	ImageRef       string          `json:"image_ref"`
	ImageID        string          `json:"image_id"`
	State          string          `json:"state"`
	Health         string          `json:"health"`
	StartedAt      *time.Time      `json:"started_at,omitempty"`
	CreatedAt      *time.Time      `json:"created_at,omitempty"`
	RestartCount   int             `json:"restart_count"`
	ComposeProject string          `json:"compose_project"`
	ComposeService string          `json:"compose_service"`
	Labels         json.RawMessage `json:"labels"`
	Ports          json.RawMessage `json:"ports"`
}

type ImageInfo struct {
	ImageID    string     `json:"image_id"`
	Repository string     `json:"repository"`
	Tag        string     `json:"tag"`
	SizeBytes  int64      `json:"size_bytes"`
	CreatedAt  *time.Time `json:"created_at,omitempty"`
	Dangling   bool       `json:"dangling"`
}

type VolumeInfo struct {
	Name       string `json:"name"`
	Driver     string `json:"driver"`
	Mountpoint string `json:"mountpoint"`
}

type NetworkInfo struct {
	NetworkID string `json:"network_id"`
	Name      string `json:"name"`
	Driver    string `json:"driver"`
	Scope     string `json:"scope"`
}

type ComposeProject struct {
	ProjectName string `json:"project_name"`
	Status      string `json:"status"`
}

type ContainerSample struct {
	TS            time.Time `json:"ts"`
	ContainerID   string    `json:"container_id"`
	CPUPct        *float64  `json:"cpu_pct"`
	MemUsedBytes  *int64    `json:"mem_used_bytes"`
	MemLimitBytes *int64    `json:"mem_limit_bytes"`
	NetRxBps      *int64    `json:"net_rx_bps"`
	NetTxBps      *int64    `json:"net_tx_bps"`
	BlkReadBps    *int64    `json:"blk_read_bps"`
	BlkWriteBps   *int64    `json:"blk_write_bps"`
	PIDs          *int      `json:"pids"`
}

// Cap Docker Engine responses so a misbehaving stream/huge inspect cannot
// grow the heap unboundedly (stats/logs paths also use LimitReader).
const maxDockerResponseBytes = 32 << 20 // 32 MiB

func dockerGET(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker"+path, nil)
	if err != nil {
		return err
	}
	res, err := dockerHTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxDockerResponseBytes))
	if err != nil {
		return err
	}
	if res.StatusCode >= 300 {
		return fmt.Errorf("docker API %s: HTTP %d: %s", path, res.StatusCode, string(body))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
}

// ProbeDocker cheaply checks Engine reachability for heartbeats without pulling
// full inventory (which used to run every metrics tick and amplified Transport leaks).
func ProbeDocker(ctx context.Context) (available, healthy bool) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := dockerGET(ctx, "/_ping", nil); err != nil {
		// Older Engines may lack /_ping; fall back to /version.
		var version struct {
			Version string `json:"Version"`
		}
		if err := dockerGET(ctx, "/version", &version); err != nil {
			return false, false
		}
	}
	return true, true
}

func CollectDocker(ctx context.Context) DockerInventory {
	var version struct {
		Version    string `json:"Version"`
		APIVersion string `json:"ApiVersion"`
	}
	if err := dockerGET(ctx, "/version", &version); err != nil {
		return DockerInventory{Available: false, DaemonHealthy: false, Error: err.Error()}
	}

	inv := DockerInventory{
		Available:     true,
		DaemonHealthy: true,
		Version:       version.Version,
		APIVersion:    version.APIVersion,
	}

	var containers []struct {
		ID      string            `json:"Id"`
		Names   []string          `json:"Names"`
		Image   string            `json:"Image"`
		ImageID string            `json:"ImageID"`
		State   string            `json:"State"`
		Status  string            `json:"Status"`
		Created int64             `json:"Created"`
		Labels  map[string]string `json:"Labels"`
		Ports   json.RawMessage   `json:"Ports"`
	}
	if err := dockerGET(ctx, "/containers/json?all=true", &containers); err != nil {
		inv.DaemonHealthy = false
		inv.Error = err.Error()
		return inv
	}

	projects := map[string]struct{}{}
	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		health := "none"
		st := strings.ToLower(c.Status)
		if strings.Contains(st, "unhealthy") {
			health = "unhealthy"
		} else if strings.Contains(st, "(healthy)") {
			health = "healthy"
		}
		labels := c.Labels
		if labels == nil {
			labels = map[string]string{}
		}
		labelsBytes, _ := json.Marshal(labels)
		ports := c.Ports
		if len(ports) == 0 {
			ports = json.RawMessage("[]")
		}
		project := labels["com.docker.compose.project"]
		service := labels["com.docker.compose.service"]
		if project != "" {
			projects[project] = struct{}{}
		}
		created := time.Unix(c.Created, 0).UTC()
		info := ContainerInfo{
			ContainerID:    c.ID,
			Name:           name,
			ImageRef:       c.Image,
			ImageID:        c.ImageID,
			State:          c.State,
			Health:         health,
			CreatedAt:      &created,
			ComposeProject: project,
			ComposeService: service,
			Labels:         labelsBytes,
			Ports:          ports,
		}

		var insp struct {
			RestartCount int `json:"RestartCount"`
			State        *struct {
				StartedAt string `json:"StartedAt"`
				Health    *struct {
					Status string `json:"Status"`
				} `json:"Health"`
			} `json:"State"`
		}
		if err := dockerGET(ctx, "/containers/"+c.ID+"/json", &insp); err == nil {
			info.RestartCount = insp.RestartCount
			if insp.State != nil {
				if insp.State.Health != nil && insp.State.Health.Status != "" {
					info.Health = insp.State.Health.Status
				}
				if t, err := time.Parse(time.RFC3339Nano, insp.State.StartedAt); err == nil && !t.IsZero() {
					ut := t.UTC()
					info.StartedAt = &ut
				}
			}
		}
		inv.Containers = append(inv.Containers, info)
	}
	for p := range projects {
		inv.ComposeProjects = append(inv.ComposeProjects, ComposeProject{ProjectName: p, Status: "detected"})
	}

	var images []struct {
		ID       string   `json:"Id"`
		RepoTags []string `json:"RepoTags"`
		Size     int64    `json:"Size"`
		Created  int64    `json:"Created"`
	}
	if err := dockerGET(ctx, "/images/json?all=true", &images); err == nil {
		for _, img := range images {
			repo, tag := "<none>", "<none>"
			dangling := len(img.RepoTags) == 0
			if len(img.RepoTags) > 0 {
				parts := strings.SplitN(img.RepoTags[0], ":", 2)
				repo = parts[0]
				if len(parts) > 1 {
					tag = parts[1]
				}
			}
			created := time.Unix(img.Created, 0).UTC()
			inv.Images = append(inv.Images, ImageInfo{
				ImageID: img.ID, Repository: repo, Tag: tag, SizeBytes: img.Size,
				CreatedAt: &created, Dangling: dangling,
			})
		}
	}

	var vols struct {
		Volumes []struct {
			Name       string `json:"Name"`
			Driver     string `json:"Driver"`
			Mountpoint string `json:"Mountpoint"`
		} `json:"Volumes"`
	}
	if err := dockerGET(ctx, "/volumes", &vols); err == nil {
		for _, v := range vols.Volumes {
			inv.Volumes = append(inv.Volumes, VolumeInfo{Name: v.Name, Driver: v.Driver, Mountpoint: v.Mountpoint})
		}
	}

	var nets []struct {
		ID     string `json:"Id"`
		Name   string `json:"Name"`
		Driver string `json:"Driver"`
		Scope  string `json:"Scope"`
	}
	if err := dockerGET(ctx, "/networks", &nets); err == nil {
		for _, n := range nets {
			inv.Networks = append(inv.Networks, NetworkInfo{
				NetworkID: n.ID, Name: n.Name, Driver: n.Driver, Scope: n.Scope,
			})
		}
	}

	return inv
}

func SampleContainerStats(ctx context.Context) ([]ContainerSample, error) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	// Fail fast if Docker is not reachable.
	if available, _ := ProbeDocker(ctx); !available {
		return nil, fmt.Errorf("docker engine unreachable")
	}
	var list []struct {
		ID string `json:"Id"`
	}
	if err := dockerGET(ctx, "/containers/json", &list); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	out := make([]ContainerSample, 0, len(list))
	for _, c := range list {
		var st struct {
			CPUStats struct {
				CPUUsage struct {
					TotalUsage  uint64   `json:"total_usage"`
					PercpuUsage []uint64 `json:"percpu_usage"`
				} `json:"cpu_usage"`
				SystemUsage uint64 `json:"system_cpu_usage"`
				OnlineCPUs  uint32 `json:"online_cpus"`
			} `json:"cpu_stats"`
			PreCPUStats struct {
				CPUUsage struct {
					TotalUsage uint64 `json:"total_usage"`
				} `json:"cpu_usage"`
				SystemUsage uint64 `json:"system_cpu_usage"`
			} `json:"precpu_stats"`
			MemoryStats struct {
				Usage uint64 `json:"usage"`
				Limit uint64 `json:"limit"`
			} `json:"memory_stats"`
			PidsStats struct {
				Current uint64 `json:"current"`
			} `json:"pids_stats"`
		}
		reqCtx, reqCancel := context.WithTimeout(ctx, 2*time.Second)
		err := dockerGET(reqCtx, "/containers/"+c.ID+"/stats?stream=false", &st)
		reqCancel()
		if err != nil {
			continue
		}
		sample := ContainerSample{TS: now, ContainerID: c.ID}
		cpu := calcCPUPercent(st.CPUStats.CPUUsage.TotalUsage, st.PreCPUStats.CPUUsage.TotalUsage,
			st.CPUStats.SystemUsage, st.PreCPUStats.SystemUsage, st.CPUStats.OnlineCPUs, len(st.CPUStats.CPUUsage.PercpuUsage))
		sample.CPUPct = &cpu
		used := int64(st.MemoryStats.Usage)
		limit := int64(st.MemoryStats.Limit)
		sample.MemUsedBytes = &used
		sample.MemLimitBytes = &limit
		pids := int(st.PidsStats.Current)
		sample.PIDs = &pids
		zero := int64(0)
		sample.NetRxBps = &zero
		sample.NetTxBps = &zero
		out = append(out, sample)
	}
	return out, nil
}

func calcCPUPercent(total, preTotal, system, preSystem uint64, online uint32, perCPU int) float64 {
	cpuDelta := float64(total - preTotal)
	systemDelta := float64(system - preSystem)
	cores := float64(online)
	if cores == 0 {
		cores = float64(perCPU)
	}
	if cores == 0 {
		cores = 1
	}
	if systemDelta > 0 && cpuDelta > 0 {
		return (cpuDelta / systemDelta) * cores * 100.0
	}
	return 0
}
