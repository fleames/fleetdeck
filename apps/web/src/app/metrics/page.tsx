"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { API_URL } from "@/lib/api";
import { pct } from "@/lib/format";

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

  useEffect(() => {
    const boot = window.setTimeout(() => {
      Promise.all([
        fetch(`${API_URL}/api/v1/overview/self`, { credentials: "include", cache: "no-store" }).then((r) =>
          r.ok ? r.json() : null,
        ),
        fetch(`${API_URL}/api/v1/servers`, { credentials: "include", cache: "no-store" }).then((r) =>
          r.ok ? r.json() : null,
        ),
      ])
        .then(([selfData, serversData]) => {
          setSelf(selfData);
          setServers(serversData?.data ?? []);
        })
        .catch((e) => setError(e instanceof Error ? e.message : "Failed to load"));
    }, 0);
    return () => window.clearTimeout(boot);
  }, []);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-semibold tracking-tight">Metrics</h1>
        <p className="mt-1 text-sm text-[var(--text-1)]">
          Platform self-metrics and latest host samples. Open a server for history charts.
        </p>
      </div>
      {error && (
        <div className="rounded-md border border-[var(--crit)]/40 bg-[var(--crit)]/10 px-3 py-2 text-sm text-[var(--crit)]">
          {error}
        </div>
      )}
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
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
      <div className="overflow-hidden rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)]">
        <table className="w-full text-left text-sm">
          <thead className="border-b border-[var(--border)] text-xs text-[var(--text-2)]">
            <tr>
              <th className="px-4 py-2 font-medium">Server</th>
              <th className="px-4 py-2 font-medium">Status</th>
              <th className="px-4 py-2 font-medium">CPU</th>
              <th className="px-4 py-2 font-medium">Memory</th>
              <th className="px-4 py-2 font-medium">Disk</th>
              <th className="px-4 py-2 font-medium">Last sample</th>
            </tr>
          </thead>
          <tbody>
            {servers.length === 0 ? (
              <tr>
                <td colSpan={6} className="px-4 py-8 text-center text-[var(--text-2)]">
                  No servers yet.
                </td>
              </tr>
            ) : (
              servers.map((s) => (
                <tr key={s.id} className="border-b border-[var(--border)] last:border-0">
                  <td className="px-4 py-2">
                    <Link href={`/servers/${s.id}`} className="text-[var(--accent)] hover:underline">
                      {s.name}
                    </Link>
                  </td>
                  <td className="px-4 py-2 text-[var(--text-1)]">{s.status}</td>
                  <td className="px-4 py-2">{fmtPct(s.metrics?.cpu_pct ?? null)}</td>
                  <td className="px-4 py-2">
                    {fmtPct(pct(s.metrics?.mem_used_bytes, s.metrics?.mem_total_bytes))}
                  </td>
                  <td className="px-4 py-2">
                    {fmtPct(pct(s.metrics?.disk_used_bytes, s.metrics?.disk_total_bytes))}
                  </td>
                  <td className="px-4 py-2 text-[var(--text-2)]">
                    {s.last_metrics_at ? new Date(s.last_metrics_at).toLocaleString() : "—"}
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] px-4 py-3">
      <div className="text-[11px] uppercase tracking-wide text-[var(--text-2)]">{label}</div>
      <div className="mt-1 text-xl font-semibold text-[var(--text-0)]">{value}</div>
    </div>
  );
}

function fmtPct(n: number | null | undefined) {
  if (n == null || Number.isNaN(n)) return "—";
  return `${n.toFixed(1)}%`;
}
