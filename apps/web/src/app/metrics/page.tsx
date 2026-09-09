"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { API_URL } from "@/lib/api";
import { pct } from "@/lib/format";
import { StatusPill } from "@/components/status-pill";
import { EmptyBlock, ErrorBanner, LoadingBlock } from "@/components/page-state";

type SelfMetrics = {
  api?: { status?: string; uptime_seconds?: number; started_at?: string };
  fleet?: {
    servers?: number;
    online_servers?: number;
    online_agents?: number;
    active_alerts?: number;
  };
  ingestion?: { server_metric_points_last_hour?: number };
};

type ServerRow = {
  id: string;
  name: string;
  status: string;
  health_state?: string;
  last_metrics_at?: string | null;
  metrics?: {
    cpu_pct?: number;
    mem_used_bytes?: number;
    mem_total_bytes?: number;
    disk_used_bytes?: number;
    disk_total_bytes?: number;
  } | null;
};

export default function MetricsPage() {
  const [self, setSelf] = useState<SelfMetrics | null>(null);
  const [servers, setServers] = useState<ServerRow[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const boot = window.setTimeout(() => {
      Promise.all([
        fetch(`${API_URL}/api/v1/overview/self`, { credentials: "include", cache: "no-store" }).then((r) => {
          if (!r.ok) throw new Error(`Self-metrics request failed (${r.status})`);
          return r.json();
        }),
        fetch(`${API_URL}/api/v1/servers`, { credentials: "include", cache: "no-store" }).then((r) => {
          if (!r.ok) throw new Error(`Servers request failed (${r.status})`);
          return r.json();
        }),
      ])
        .then(([selfData, serversData]) => {
          setSelf(selfData);
          setServers(serversData?.data ?? []);
          setError(null);
        })
        .catch((e) => setError(e instanceof Error ? e.message : "Failed to load"))
        .finally(() => setLoading(false));
    }, 0);
    return () => window.clearTimeout(boot);
  }, []);

  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <div>
        <div className="text-[11px] font-medium uppercase tracking-[0.18em] text-[var(--text-2)]">
          Observability
        </div>
        <h1 className="mt-1 text-3xl font-semibold tracking-tight">Metrics</h1>
        <p className="mt-1 text-sm text-[var(--text-1)]">
          Platform self-metrics and latest host samples. Open a server for history charts.
        </p>
      </div>
      {error && <ErrorBanner message={error} />}
      {loading && !self && servers.length === 0 && !error ? (
        <LoadingBlock label="Loading metrics" />
      ) : (
        <>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4" aria-label="Platform metrics">
            <Stat label="API uptime" value={`${self?.api?.uptime_seconds ?? "—"}s`} />
            <Stat
              label="Ingest (1h)"
              value={String(self?.ingestion?.server_metric_points_last_hour ?? "—")}
            />
            <Stat
              label="Servers online"
              value={`${self?.fleet?.online_servers ?? "—"} / ${self?.fleet?.servers ?? "—"}`}
            />
            <Stat label="Active alerts" value={String(self?.fleet?.active_alerts ?? "—")} />
          </div>
          {servers.length === 0 ? (
            <EmptyBlock>No servers yet — enroll a host to see latest samples.</EmptyBlock>
          ) : (
            <div className="overflow-hidden rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)]">
              <table className="w-full text-left text-sm">
                <thead className="border-b border-[var(--border)] bg-[var(--bg-2)] text-[11px] uppercase tracking-[0.12em] text-[var(--text-2)]">
                  <tr>
                    <th className="px-4 py-3 font-medium">Server</th>
                    <th className="px-4 py-3 font-medium">Status</th>
                    <th className="px-4 py-3 font-medium">CPU</th>
                    <th className="px-4 py-3 font-medium">Memory</th>
                    <th className="px-4 py-3 font-medium">Disk</th>
                    <th className="px-4 py-3 font-medium">Last sample</th>
                  </tr>
                </thead>
                <tbody>
                  {servers.map((s) => (
                    <tr key={s.id} className="border-b border-[var(--border)] last:border-0">
                      <td className="px-4 py-3">
                        <Link
                          href={`/servers/${s.id}`}
                          className="text-[var(--accent)] underline-offset-2 hover:underline"
                        >
                          {s.name}
                        </Link>
                      </td>
                      <td className="px-4 py-3">
                        <StatusPill state={s.health_state || s.status} />
                      </td>
                      <td className="px-4 py-3 font-[family-name:var(--font-mono-family)]">
                        {fmtPct(s.metrics?.cpu_pct ?? null)}
                      </td>
                      <td className="px-4 py-3 font-[family-name:var(--font-mono-family)]">
                        {fmtPct(pct(s.metrics?.mem_used_bytes, s.metrics?.mem_total_bytes))}
                      </td>
                      <td className="px-4 py-3 font-[family-name:var(--font-mono-family)]">
                        {fmtPct(pct(s.metrics?.disk_used_bytes, s.metrics?.disk_total_bytes))}
                      </td>
                      <td className="px-4 py-3 text-[var(--text-2)]">
                        {s.last_metrics_at ? new Date(s.last_metrics_at).toLocaleString() : "—"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] px-4 py-3">
      <div className="text-[11px] uppercase tracking-[0.12em] text-[var(--text-2)]">{label}</div>
      <div className="mt-1 font-[family-name:var(--font-mono-family)] text-xl text-[var(--text-0)]">
        {value}
      </div>
    </div>
  );
}

function fmtPct(n: number | null | undefined) {
  if (n == null || Number.isNaN(n)) return "—";
  return `${n.toFixed(1)}%`;
}
