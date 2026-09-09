package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const metricsInsertChunk = 40

func (s *Server) tryAcquireIngest() bool {
	select {
	case s.ingestSem <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *Server) releaseIngest() {
	select {
	case <-s.ingestSem:
	default:
	}
}

func insertHostMetricsBatch(ctx context.Context, tx pgx.Tx, serverID uuid.UUID, samples []metricSample) (time.Time, error) {
	var lastTS time.Time
	if len(samples) == 0 {
		return lastTS, nil
	}
	type row struct {
		ts time.Time
		m  metricSample
	}
	rows := make([]row, 0, len(samples))
	for _, m := range samples {
		ts := m.TS
		if ts.IsZero() {
			ts = time.Now().UTC()
		}
		if ts.After(lastTS) {
			lastTS = ts
		}
		rows = append(rows, row{ts: ts, m: m})
	}
	for i := 0; i < len(rows); i += metricsInsertChunk {
		end := i + metricsInsertChunk
		if end > len(rows) {
			end = len(rows)
		}
		chunk := rows[i:end]
		var b strings.Builder
		args := make([]any, 0, len(chunk)*17)
		b.WriteString(`INSERT INTO server_metrics_raw (
			ts, server_id, cpu_pct, load1, load5, load15,
			mem_used_bytes, mem_available_bytes, mem_cached_bytes, swap_used_bytes,
			disk_used_bytes, disk_total_bytes, net_rx_bps, net_tx_bps, net_rx_errs, net_tx_errs, uptime_seconds
		) VALUES `)
		for j, r := range chunk {
			if j > 0 {
				b.WriteByte(',')
			}
			base := j * 17
			fmt.Fprintf(&b, "($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
				base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9, base+10,
				base+11, base+12, base+13, base+14, base+15, base+16, base+17)
			m := r.m
			args = append(args,
				r.ts, serverID, m.CPUPct, m.Load1, m.Load5, m.Load15,
				m.MemUsedBytes, m.MemAvailableBytes, m.MemCachedBytes, m.SwapUsedBytes,
				m.DiskUsedBytes, m.DiskTotalBytes, m.NetRxBps, m.NetTxBps, m.NetRxErrs, m.NetTxErrs, m.UptimeSeconds,
			)
		}
		b.WriteString(` ON CONFLICT (server_id, ts) DO UPDATE SET
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
			uptime_seconds=EXCLUDED.uptime_seconds`)
		if _, err := tx.Exec(ctx, b.String(), args...); err != nil {
			return lastTS, err
		}
	}
	return lastTS, nil
}

func insertContainerMetricsBatch(ctx context.Context, tx pgx.Tx, serverID uuid.UUID, samples []containerMetricSample) error {
	if len(samples) == 0 {
		return nil
	}
	ids := make([]string, 0, len(samples))
	seen := map[string]struct{}{}
	for _, m := range samples {
		if m.ContainerID == "" {
			continue
		}
		if _, ok := seen[m.ContainerID]; ok {
			continue
		}
		seen[m.ContainerID] = struct{}{}
		ids = append(ids, m.ContainerID)
	}
	idMap := map[string]uuid.UUID{}
	if len(ids) > 0 {
		rows, err := tx.Query(ctx, `
			SELECT container_id, id FROM containers WHERE server_id=$1 AND container_id = ANY($2)`,
			serverID, ids)
		if err != nil {
			return err
		}
		for rows.Next() {
			var cid string
			var id uuid.UUID
			if err := rows.Scan(&cid, &id); err != nil {
				rows.Close()
				return err
			}
			idMap[cid] = id
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}

	type row struct {
		ts  time.Time
		cid uuid.UUID
		m   containerMetricSample
	}
	ready := make([]row, 0, len(samples))
	for _, m := range samples {
		cid, ok := idMap[m.ContainerID]
		if !ok {
			continue // inventory may arrive later
		}
		ts := m.TS
		if ts.IsZero() {
			ts = time.Now().UTC()
		}
		ready = append(ready, row{ts: ts, cid: cid, m: m})
	}
	for i := 0; i < len(ready); i += metricsInsertChunk {
		end := i + metricsInsertChunk
		if end > len(ready) {
			end = len(ready)
		}
		chunk := ready[i:end]
		var b strings.Builder
		args := make([]any, 0, len(chunk)*11)
		b.WriteString(`INSERT INTO container_metrics_raw (
			ts, server_id, container_id, cpu_pct, mem_used_bytes, mem_limit_bytes,
			net_rx_bps, net_tx_bps, blk_read_bps, blk_write_bps, pids
		) VALUES `)
		for j, r := range chunk {
			if j > 0 {
				b.WriteByte(',')
			}
			base := j * 11
			fmt.Fprintf(&b, "($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
				base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9, base+10, base+11)
			m := r.m
			args = append(args,
				r.ts, serverID, r.cid, m.CPUPct, m.MemUsedBytes, m.MemLimitBytes,
				m.NetRxBps, m.NetTxBps, m.BlkReadBps, m.BlkWriteBps, m.PIDs,
			)
		}
		b.WriteString(` ON CONFLICT (container_id, ts) DO UPDATE SET
			cpu_pct=EXCLUDED.cpu_pct,
			mem_used_bytes=EXCLUDED.mem_used_bytes,
			mem_limit_bytes=EXCLUDED.mem_limit_bytes,
			net_rx_bps=EXCLUDED.net_rx_bps,
			net_tx_bps=EXCLUDED.net_tx_bps,
			blk_read_bps=EXCLUDED.blk_read_bps,
			blk_write_bps=EXCLUDED.blk_write_bps,
			pids=EXCLUDED.pids`)
		if _, err := tx.Exec(ctx, b.String(), args...); err != nil {
			return err
		}
	}
	return nil
}
