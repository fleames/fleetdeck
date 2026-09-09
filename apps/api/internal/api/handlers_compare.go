package api

import (
	"net/http"
	"time"

	"github.com/fleetdeck/fleetdeck/apps/api/internal/httpx"
	"github.com/google/uuid"
)

func (s *Server) handleCompareServers(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ServerIDs []string `json:"server_ids"`
	}
	if err := httpx.Decode(r, &body); err != nil || len(body.ServerIDs) < 2 {
		httpx.Error(w, http.StatusBadRequest, "validation", "Provide at least two server_ids.")
		return
	}
	if len(body.ServerIDs) > 8 {
		httpx.Error(w, http.StatusBadRequest, "validation", "Compare supports at most 8 servers.")
		return
	}

	ids := make([]uuid.UUID, 0, len(body.ServerIDs))
	for _, raw := range body.ServerIDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "validation", "Invalid server id.")
			return
		}
		ids = append(ids, id)
	}

	type row struct {
		ID            uuid.UUID      `json:"id"`
		Name          string         `json:"name"`
		Status        string         `json:"status"`
		HealthState   string         `json:"health_state"`
		Metrics       *latestMetrics `json:"metrics,omitempty"`
		RunningCount  int            `json:"running_containers"`
		CapacityNotes []string       `json:"capacity_notes"`
	}
	out := make([]row, 0, len(ids))
	for _, id := range ids {
		var item row
		var m latestMetrics
		err := s.pool.QueryRow(r.Context(), `
			SELECT s.id, s.name, s.status, s.health_state,
			       m.cpu_pct, m.mem_used_bytes,
			       CASE WHEN m.mem_used_bytes IS NOT NULL AND m.mem_available_bytes IS NOT NULL
			            THEN m.mem_used_bytes + m.mem_available_bytes ELSE NULL END,
			       m.disk_used_bytes, m.disk_total_bytes, m.net_rx_bps, m.net_tx_bps, m.uptime_seconds, m.ts,
			       (SELECT COUNT(*) FROM containers c WHERE c.server_id=s.id AND c.state='running')
			FROM servers s
			LEFT JOIN LATERAL (
				SELECT * FROM server_metrics_raw sm WHERE sm.server_id=s.id ORDER BY sm.ts DESC LIMIT 1
			) m ON true
			WHERE s.id=$1`, id,
		).Scan(&item.ID, &item.Name, &item.Status, &item.HealthState,
			&m.CPUPct, &m.MemUsedBytes, &m.MemTotalApprox, &m.DiskUsedBytes, &m.DiskTotalBytes,
			&m.NetRxBps, &m.NetTxBps, &m.UptimeSeconds, &m.LastUpdated, &item.RunningCount)
		if err != nil {
			httpx.Error(w, http.StatusNotFound, "not_found", "One or more servers were not found.")
			return
		}
		if m.LastUpdated != nil {
			item.Metrics = &m
		}
		item.CapacityNotes = capacityNotes(item.Metrics)
		out = append(out, item)
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"compared_at": time.Now().UTC(),
		"data":        out,
	})
}

func capacityNotes(m *latestMetrics) []string {
	notes := []string{}
	if m == nil {
		return []string{"No recent metrics"}
	}
	if m.CPUPct != nil && *m.CPUPct >= 85 {
		notes = append(notes, "High CPU pressure")
	}
	if m.MemUsedBytes != nil && m.MemTotalApprox != nil && *m.MemTotalApprox > 0 {
		pct := float64(*m.MemUsedBytes) / float64(*m.MemTotalApprox) * 100
		if pct >= 90 {
			notes = append(notes, "High memory pressure")
		}
	}
	if m.DiskUsedBytes != nil && m.DiskTotalBytes != nil && *m.DiskTotalBytes > 0 {
		pct := float64(*m.DiskUsedBytes) / float64(*m.DiskTotalBytes) * 100
		if pct >= 90 {
			notes = append(notes, "Disk nearing capacity")
		}
	}
	if len(notes) == 0 {
		notes = append(notes, "Within normal operating range")
	}
	return notes
}
