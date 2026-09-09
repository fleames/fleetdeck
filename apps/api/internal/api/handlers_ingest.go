package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/fleetdeck/fleetdeck/apps/api/internal/httpx"
	"github.com/google/uuid"
)

type metricSample struct {
	TS                 time.Time `json:"ts"`
	CPUPct             *float64  `json:"cpu_pct"`
	Load1              *float64  `json:"load1"`
	Load5              *float64  `json:"load5"`
	Load15             *float64  `json:"load15"`
	MemUsedBytes       *int64    `json:"mem_used_bytes"`
	MemAvailableBytes  *int64    `json:"mem_available_bytes"`
	MemCachedBytes     *int64    `json:"mem_cached_bytes"`
	SwapUsedBytes      *int64    `json:"swap_used_bytes"`
	DiskUsedBytes      *int64    `json:"disk_used_bytes"`
	DiskTotalBytes     *int64    `json:"disk_total_bytes"`
	NetRxBps           *int64    `json:"net_rx_bps"`
	NetTxBps           *int64    `json:"net_tx_bps"`
	NetRxErrs          *int64    `json:"net_rx_errs"`
	NetTxErrs          *int64    `json:"net_tx_errs"`
	UptimeSeconds      *int64    `json:"uptime_seconds"`
}

type containerMetricSample struct {
	TS             time.Time `json:"ts"`
	ContainerID    string    `json:"container_id"`
	CPUPct         *float64  `json:"cpu_pct"`
	MemUsedBytes   *int64    `json:"mem_used_bytes"`
	MemLimitBytes  *int64    `json:"mem_limit_bytes"`
	NetRxBps       *int64    `json:"net_rx_bps"`
	NetTxBps       *int64    `json:"net_tx_bps"`
	BlkReadBps     *int64    `json:"blk_read_bps"`
	BlkWriteBps    *int64    `json:"blk_write_bps"`
	PIDs           *int      `json:"pids"`
}

type metricsIngestRequest struct {
	SchemaVersion int                     `json:"schema_version"`
	AgentVersion  string                  `json:"agent_version"`
	SentAt        time.Time               `json:"sent_at"`
	Host          []metricSample          `json:"host"`
	Containers    []containerMetricSample `json:"containers"`
}

func (s *Server) handleAgentMetrics(w http.ResponseWriter, r *http.Request) {
	ident := r.Context().Value(ctxAgent).(agentIdentity)
	var body metricsIngestRequest
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "Invalid metrics payload.")
		return
	}
	if body.SchemaVersion != 0 && body.SchemaVersion != 1 {
		httpx.Error(w, http.StatusBadRequest, "unsupported_schema", "Unsupported metrics schema_version.")
		return
	}
	if len(body.Host) == 0 && len(body.Containers) == 0 {
		httpx.Error(w, http.StatusBadRequest, "validation", "Metrics batch is empty.")
		return
	}
	if len(body.Host) > 120 || len(body.Containers) > 2000 {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "payload_too_large", "Metrics batch exceeds limits.")
		return
	}

	ctx := r.Context()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not ingest metrics.")
		return
	}
	defer tx.Rollback(ctx)

	var lastTS time.Time
	for _, m := range body.Host {
		ts := m.TS
		if ts.IsZero() {
			ts = time.Now().UTC()
		}
		if ts.After(lastTS) {
			lastTS = ts
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO server_metrics_raw (
				ts, server_id, cpu_pct, load1, load5, load15,
				mem_used_bytes, mem_available_bytes, mem_cached_bytes, swap_used_bytes,
				disk_used_bytes, disk_total_bytes, net_rx_bps, net_tx_bps, net_rx_errs, net_tx_errs, uptime_seconds
			) VALUES (
				$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17
			)
			ON CONFLICT (server_id, ts) DO UPDATE SET
				cpu_pct=EXCLUDED.cpu_pct,
				load1=EXCLUDED.load1,
				load5=EXCLUDED.load5,
				load15=EXCLUDED.load15,
				mem_used_bytes=EXCLUDED.mem_used_bytes,
				mem_available_bytes=EXCLUDED.mem_available_bytes,
				mem_cached_bytes=EXCLUDED.mem_cached_bytes,
				swap_used_bytes=EXCLUDED.swap_used_bytes,
				disk_used_bytes=EXCLUDED.disk_used_bytes,
				disk_total_bytes=EXCLUDED.disk_total_bytes,
				net_rx_bps=EXCLUDED.net_rx_bps,
				net_tx_bps=EXCLUDED.net_tx_bps,
				net_rx_errs=EXCLUDED.net_rx_errs,
				net_tx_errs=EXCLUDED.net_tx_errs,
				uptime_seconds=EXCLUDED.uptime_seconds`,
			ts, ident.ServerID, m.CPUPct, m.Load1, m.Load5, m.Load15,
			m.MemUsedBytes, m.MemAvailableBytes, m.MemCachedBytes, m.SwapUsedBytes,
			m.DiskUsedBytes, m.DiskTotalBytes, m.NetRxBps, m.NetTxBps, m.NetRxErrs, m.NetTxErrs, m.UptimeSeconds,
		)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not store host metrics.")
			return
		}
	}

	for _, m := range body.Containers {
		if m.ContainerID == "" {
			continue
		}
		var containerUUID uuid.UUID
		err := tx.QueryRow(ctx, `
			SELECT id FROM containers WHERE server_id=$1 AND container_id=$2`,
			ident.ServerID, m.ContainerID,
		).Scan(&containerUUID)
		if err != nil {
			continue // inventory may arrive later
		}
		ts := m.TS
		if ts.IsZero() {
			ts = time.Now().UTC()
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO container_metrics_raw (
				ts, server_id, container_id, cpu_pct, mem_used_bytes, mem_limit_bytes,
				net_rx_bps, net_tx_bps, blk_read_bps, blk_write_bps, pids
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			ON CONFLICT (container_id, ts) DO UPDATE SET
				cpu_pct=EXCLUDED.cpu_pct,
				mem_used_bytes=EXCLUDED.mem_used_bytes,
				mem_limit_bytes=EXCLUDED.mem_limit_bytes,
				net_rx_bps=EXCLUDED.net_rx_bps,
				net_tx_bps=EXCLUDED.net_tx_bps,
				blk_read_bps=EXCLUDED.blk_read_bps,
				blk_write_bps=EXCLUDED.blk_write_bps,
				pids=EXCLUDED.pids`,
			ts, ident.ServerID, containerUUID, m.CPUPct, m.MemUsedBytes, m.MemLimitBytes,
			m.NetRxBps, m.NetTxBps, m.BlkReadBps, m.BlkWriteBps, m.PIDs,
		)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not store container metrics.")
			return
		}
	}

	if !lastTS.IsZero() {
		_, _ = tx.Exec(ctx, `
			UPDATE servers SET last_metrics_at=$2, last_seen_at=now(), status='online', updated_at=now()
			WHERE id=$1`, ident.ServerID, lastTS)
	}
	_, _ = tx.Exec(ctx, `
		UPDATE agents SET last_heartbeat_at=now(), status='online',
		agent_version=COALESCE(NULLIF($2,''), agent_version), updated_at=now()
		WHERE id=$1`, ident.AgentID, body.AgentVersion)

	if err := tx.Commit(ctx); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not ingest metrics.")
		return
	}
	s.hub.Broadcast("metrics.batch", map[string]any{
		"server_id":         ident.ServerID,
		"host_samples":      len(body.Host),
		"container_samples": len(body.Containers),
	})
	s.hub.Broadcast("servers.updated", map[string]any{"server_id": ident.ServerID})
	httpx.JSON(w, http.StatusAccepted, map[string]any{
		"ok":                true,
		"host_samples":      len(body.Host),
		"container_samples": len(body.Containers),
	})
}

type inventoryRequest struct {
	SchemaVersion int `json:"schema_version"`
	AgentVersion  string `json:"agent_version"`
	Host          struct {
		Hostname   string `json:"hostname"`
		OSName     string `json:"os_name"`
		OSVersion  string `json:"os_version"`
		Arch       string `json:"arch"`
		PrimaryIP  string `json:"primary_address"`
	} `json:"host"`
	Docker *struct {
		Available     bool   `json:"available"`
		Version       string `json:"version"`
		APIVersion    string `json:"api_version"`
		DaemonHealthy bool   `json:"daemon_healthy"`
	} `json:"docker"`
	Containers []struct {
		ContainerID    string          `json:"container_id"`
		Name           string          `json:"name"`
		ImageRef       string          `json:"image_ref"`
		ImageID        string          `json:"image_id"`
		State          string          `json:"state"`
		Health         string          `json:"health"`
		StartedAt      *time.Time      `json:"started_at"`
		CreatedAt      *time.Time      `json:"created_at"`
		RestartCount   int             `json:"restart_count"`
		ComposeProject string          `json:"compose_project"`
		ComposeService string          `json:"compose_service"`
		Labels         json.RawMessage `json:"labels"`
		Ports          json.RawMessage `json:"ports"`
	} `json:"containers"`
	Images []struct {
		ImageID    string     `json:"image_id"`
		Repository string     `json:"repository"`
		Tag        string     `json:"tag"`
		SizeBytes  int64      `json:"size_bytes"`
		CreatedAt  *time.Time `json:"created_at"`
		Dangling   bool       `json:"dangling"`
	} `json:"images"`
	Volumes []struct {
		Name       string `json:"name"`
		Driver     string `json:"driver"`
		Mountpoint string `json:"mountpoint"`
	} `json:"volumes"`
	Networks []struct {
		NetworkID string `json:"network_id"`
		Name      string `json:"name"`
		Driver    string `json:"driver"`
		Scope     string `json:"scope"`
	} `json:"networks"`
	ComposeProjects []struct {
		ProjectName string `json:"project_name"`
		Status      string `json:"status"`
	} `json:"compose_projects"`
}

func (s *Server) handleAgentInventory(w http.ResponseWriter, r *http.Request) {
	ident := r.Context().Value(ctxAgent).(agentIdentity)
	var body inventoryRequest
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_body", "Invalid inventory payload.")
		return
	}

	ctx := r.Context()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not ingest inventory.")
		return
	}
	defer tx.Rollback(ctx)

	dockerAvailable := body.Docker != nil && body.Docker.Available
	_, err = tx.Exec(ctx, `
		UPDATE servers SET
			hostname=COALESCE(NULLIF($2,''), hostname),
			os_name=COALESCE(NULLIF($3,''), os_name),
			os_version=COALESCE(NULLIF($4,''), os_version),
			arch=COALESCE(NULLIF($5,''), arch),
			primary_address=COALESCE(NULLIF($6,''), primary_address),
			docker_available=$7,
			status='online',
			health_state=CASE WHEN $7 AND COALESCE($8,false)=false THEN 'warning' ELSE 'healthy' END,
			last_seen_at=now(),
			updated_at=now()
		WHERE id=$1`,
		ident.ServerID, body.Host.Hostname, body.Host.OSName, body.Host.OSVersion, body.Host.Arch, body.Host.PrimaryIP,
		dockerAvailable, body.Docker != nil && body.Docker.DaemonHealthy,
	)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not update server inventory.")
		return
	}

	_, _ = tx.Exec(ctx, `
		UPDATE agents SET agent_version=COALESCE(NULLIF($2,''), agent_version),
			os=COALESCE(NULLIF($3,''), os), arch=COALESCE(NULLIF($4,''), arch),
			status='online', last_heartbeat_at=now(), updated_at=now()
		WHERE id=$1`, ident.AgentID, body.AgentVersion, body.Host.OSName, body.Host.Arch)

	if body.Docker != nil && body.Docker.Available {
		_, err = tx.Exec(ctx, `
			INSERT INTO docker_hosts (server_id, docker_version, api_version, daemon_healthy, updated_at)
			VALUES ($1,$2,$3,$4,now())
			ON CONFLICT (server_id) DO UPDATE SET
				docker_version=EXCLUDED.docker_version,
				api_version=EXCLUDED.api_version,
				daemon_healthy=EXCLUDED.daemon_healthy,
				updated_at=now()`,
			ident.ServerID, body.Docker.Version, body.Docker.APIVersion, body.Docker.DaemonHealthy,
		)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not upsert docker host.")
			return
		}
	}

	seenContainers := make([]string, 0, len(body.Containers))
	for _, c := range body.Containers {
		if c.ContainerID == "" {
			continue
		}
		labels := c.Labels
		if len(labels) == 0 {
			labels = json.RawMessage(`{}`)
		}
		ports := c.Ports
		if len(ports) == 0 {
			ports = json.RawMessage(`[]`)
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO containers (
				server_id, container_id, name, image_ref, image_id, state, health,
				started_at, container_created_at, restart_count, compose_project, compose_service,
				labels, ports, updated_at, last_seen_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13::jsonb,$14::jsonb,now(),now())
			ON CONFLICT (server_id, container_id) DO UPDATE SET
				name=EXCLUDED.name, image_ref=EXCLUDED.image_ref, image_id=EXCLUDED.image_id,
				state=EXCLUDED.state, health=EXCLUDED.health, started_at=EXCLUDED.started_at,
				container_created_at=EXCLUDED.container_created_at, restart_count=EXCLUDED.restart_count,
				compose_project=EXCLUDED.compose_project, compose_service=EXCLUDED.compose_service,
				labels=EXCLUDED.labels, ports=EXCLUDED.ports, updated_at=now(), last_seen_at=now()`,
			ident.ServerID, c.ContainerID, c.Name, c.ImageRef, c.ImageID, nullIfEmpty(c.State, "unknown"),
			nullIfEmpty(c.Health, "none"), c.StartedAt, c.CreatedAt, c.RestartCount, c.ComposeProject, c.ComposeService,
			string(labels), string(ports),
		)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not upsert containers.")
			return
		}
		seenContainers = append(seenContainers, c.ContainerID)
	}

	for _, img := range body.Images {
		if img.ImageID == "" {
			continue
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO images (server_id, image_id, repository, tag, size_bytes, created_at, dangling, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,now())
			ON CONFLICT (server_id, image_id) DO UPDATE SET
				repository=EXCLUDED.repository, tag=EXCLUDED.tag, size_bytes=EXCLUDED.size_bytes,
				created_at=EXCLUDED.created_at, dangling=EXCLUDED.dangling, updated_at=now()`,
			ident.ServerID, img.ImageID, img.Repository, img.Tag, img.SizeBytes, img.CreatedAt, img.Dangling,
		)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not upsert images.")
			return
		}
	}

	for _, v := range body.Volumes {
		if v.Name == "" {
			continue
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO volumes (server_id, name, driver, mountpoint, updated_at)
			VALUES ($1,$2,$3,$4,now())
			ON CONFLICT (server_id, name) DO UPDATE SET
				driver=EXCLUDED.driver, mountpoint=EXCLUDED.mountpoint, updated_at=now()`,
			ident.ServerID, v.Name, v.Driver, v.Mountpoint,
		)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not upsert volumes.")
			return
		}
	}

	for _, n := range body.Networks {
		if n.NetworkID == "" {
			continue
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO networks (server_id, network_id, name, driver, scope, updated_at)
			VALUES ($1,$2,$3,$4,$5,now())
			ON CONFLICT (server_id, network_id) DO UPDATE SET
				name=EXCLUDED.name, driver=EXCLUDED.driver, scope=EXCLUDED.scope, updated_at=now()`,
			ident.ServerID, n.NetworkID, n.Name, n.Driver, n.Scope,
		)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not upsert networks.")
			return
		}
	}

	composeNames := map[string]string{}
	for _, c := range body.Containers {
		if c.ComposeProject != "" {
			composeNames[c.ComposeProject] = "detected"
		}
	}
	for _, p := range body.ComposeProjects {
		if p.ProjectName != "" {
			composeNames[p.ProjectName] = nullIfEmpty(p.Status, "detected")
		}
	}
	for name, status := range composeNames {
		_, err = tx.Exec(ctx, `
			INSERT INTO compose_projects (server_id, project_name, status, updated_at)
			VALUES ($1,$2,$3,now())
			ON CONFLICT (server_id, project_name) DO UPDATE SET status=EXCLUDED.status, updated_at=now()`,
			ident.ServerID, name, status,
		)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", "Could not upsert compose projects.")
			return
		}
	}

	_ = seenContainers
	if err := tx.Commit(ctx); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", "Could not ingest inventory.")
		return
	}
	s.hub.Broadcast("servers.updated", map[string]any{"server_id": ident.ServerID, "inventory": true})
	s.hub.Broadcast("docker.updated", map[string]any{"server_id": ident.ServerID})
	httpx.JSON(w, http.StatusAccepted, map[string]any{"ok": true})
}

func nullIfEmpty(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
