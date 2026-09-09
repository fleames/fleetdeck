"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { api, Overview, Server, API_URL, apiFetch } from "@/lib/api";
import { StatusPill, agentVersionOutdated } from "@/components/status-pill";
import {
  formatBps,
  formatUptime,
  freshnessLabel,
  pct,
} from "@/lib/format";
import { ResourceBar } from "@/components/resource-bar";

import { useRealtime } from "@/lib/realtime";

type WidgetID = "health" | "counts" | "servers";
type Widget = { id: WidgetID; visible: boolean };

const DEFAULT_LAYOUT: Widget[] = [
  { id: "health", visible: true },
  { id: "counts", visible: true },
  { id: "servers", visible: true },
];

const LABELS: Record<WidgetID, string> = {
  health: "Health score",
  counts: "Fleet counts",
  servers: "Server cards",
};

export default function DashboardPage() {
  const [overview, setOverview] = useState<Overview | null>(null);
  const [servers, setServers] = useState<Server[]>([]);
  const [currentAgentVersion, setCurrentAgentVersion] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [tick, setTick] = useState(0);
  const [layout, setLayout] = useState<Widget[]>(DEFAULT_LAYOUT);
  const [customize, setCustomize] = useState(false);
  const [layoutMsg, setLayoutMsg] = useState<string | null>(null);

  useRealtime((type) => {
    if (type === "servers.updated" || type === "metrics.batch" || type === "overview.delta" || type === "alerts.updated") {
      setTick((t) => t + 1);
    }
  });

  useEffect(() => {
    const boot = window.setTimeout(() => {
      fetch(`${API_URL}/api/v1/dashboard/layout`, { credentials: "include", cache: "no-store" })
        .then((r) => (r.ok ? r.json() : null))
        .then((data) => {
          if (!data?.layout || !Array.isArray(data.layout)) return;
          const next = normalizeLayout(data.layout);
          setLayout(next);
        })
        .catch(() => undefined);
    }, 0);
    return () => window.clearTimeout(boot);
  }, []);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      try {
        const [ov, sv] = await Promise.all([api.overview(), api.servers()]);
        if (cancelled) return;
        setOverview(ov);
        setServers(sv.data);
        setCurrentAgentVersion(sv.meta.current_agent_version ?? null);
        setError(null);
      } catch (e) {
        if (!cancelled) setError(e instanceof Error ? e.message : "Failed to load");
      }
    }
    const boot = window.setTimeout(() => {
      void load();
    }, 0);
    const id = window.setInterval(() => {
      void load();
    }, 8000);
    return () => {
      cancelled = true;
      window.clearTimeout(boot);
      window.clearInterval(id);
    };
  }, [tick]);

  const visible = useMemo(() => layout.filter((w) => w.visible).map((w) => w.id), [layout]);

  async function saveLayout(next: Widget[]) {
    setLayout(next);
    setLayoutMsg(null);
    try {
      const res = await apiFetch(`/api/v1/dashboard/layout`, {
        method: "PUT",
        body: JSON.stringify({ name: "Default", layout: next }),
      });
      if (!res.ok) throw new Error("Save failed");
      setLayoutMsg("Layout saved.");
    } catch {
      setLayoutMsg("Could not save layout.");
    }
  }

  function toggleWidget(id: WidgetID) {
    const next = layout.map((w) => (w.id === id ? { ...w, visible: !w.visible } : w));
    void saveLayout(next);
  }

  function moveWidget(id: WidgetID, dir: -1 | 1) {
    const idx = layout.findIndex((w) => w.id === id);
    const j = idx + dir;
    if (idx < 0 || j < 0 || j >= layout.length) return;
    const next = [...layout];
    const tmp = next[idx];
    next[idx] = next[j];
    next[j] = tmp;
    void saveLayout(next);
  }

  if (error) {
    return <ErrorPanel message={error} />;
  }

  if (!overview) {
    return <SkeletonDashboard />;
  }

  const empty = overview.counts.servers === 0;

  return (
    <div className="mx-auto max-w-7xl space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="text-[11px] font-medium uppercase tracking-[0.18em] text-[var(--text-2)]">
            Infrastructure
          </div>
          <h1 className="mt-1 text-3xl font-semibold tracking-tight">Dashboard</h1>
          <p className="mt-1 text-sm text-[var(--text-1)]">
            Is everything okay? Health score is explainable and based on live registry state — never mock data.
          </p>
        </div>
        <button
          type="button"
          className="rounded-md border border-[var(--border)] px-3 py-1.5 text-xs text-[var(--text-1)] hover:bg-[var(--bg-2)]"
          onClick={() => setCustomize((v) => !v)}
        >
          {customize ? "Done" : "Customize"}
        </button>
      </div>

      {customize && (
        <section className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-4">
          <h2 className="text-sm font-medium">Dashboard widgets</h2>
          <p className="mt-1 text-xs text-[var(--text-2)]">
            Show, hide, and reorder sections. Layout is stored per user.
          </p>
          <ul className="mt-3 space-y-2">
            {layout.map((w) => (
              <li
                key={w.id}
                className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-[var(--border)] px-3 py-2 text-sm"
              >
                <label className="flex items-center gap-2">
                  <input type="checkbox" checked={w.visible} onChange={() => toggleWidget(w.id)} />
                  {LABELS[w.id]}
                </label>
                <div className="flex gap-1">
                  <button
                    type="button"
                    className="rounded border border-[var(--border)] px-2 py-0.5 text-xs"
                    onClick={() => moveWidget(w.id, -1)}
                  >
                    Up
                  </button>
                  <button
                    type="button"
                    className="rounded border border-[var(--border)] px-2 py-0.5 text-xs"
                    onClick={() => moveWidget(w.id, 1)}
                  >
                    Down
                  </button>
                </div>
              </li>
            ))}
          </ul>
          {layoutMsg && <p className="mt-2 text-xs text-[var(--text-2)]">{layoutMsg}</p>}
        </section>
      )}

      {layout
        .filter((w) => w.visible)
        .map((w) => {
          if (w.id === "health" || w.id === "counts") {
            if (!visible.includes("health") && !visible.includes("counts")) return null;
            if (w.id === "counts" && visible.includes("health")) return null;
            const showHealth = visible.includes("health");
            const showCounts = visible.includes("counts");
            return (
              <section key="health-counts" className="grid gap-4 md:grid-cols-[1.2fr_1fr]">
                {showHealth && (
                  <div className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-5">
                    <div className="text-xs uppercase tracking-[0.14em] text-[var(--text-2)]">
                      Infrastructure health
                    </div>
                    <div className="mt-3 flex items-end gap-3">
                      <div className="font-[family-name:var(--font-mono-family)] text-5xl font-medium">
                        {overview.health.score}
                      </div>
                      <div className="pb-2 text-sm text-[var(--text-2)]">/ {overview.health.max}</div>
                    </div>
                    <div className="mt-4 h-2 overflow-hidden rounded-full bg-[var(--bg-3)]">
                      <div
                        className="h-full rounded-full bg-[var(--ok)] transition-all"
                        style={{ width: `${overview.health.score}%` }}
                      />
                    </div>
                    <ul className="mt-4 space-y-2 text-sm text-[var(--text-1)]">
                      {overview.health.factors.map((f) => (
                        <li key={f.code} className="flex justify-between gap-4">
                          <span>{f.detail}</span>
                          <span className="font-[family-name:var(--font-mono-family)] text-[var(--text-2)]">
                            {f.impact === 0 ? "—" : f.impact}
                          </span>
                        </li>
                      ))}
                    </ul>
                  </div>
                )}
                {showCounts && (
                  <div className={`grid grid-cols-2 gap-3 ${showHealth ? "" : "md:col-span-2 md:grid-cols-3"}`}>
                    <Stat label="Servers" value={overview.counts.servers} />
                    <Stat label="Online" value={overview.counts.online} tone="ok" />
                    <Stat label="Warnings" value={overview.counts.warnings} tone="warn" />
                    <Stat label="Critical" value={overview.counts.critical_alerts} tone="crit" />
                    <Stat label="Containers" value={overview.counts.running_containers} />
                    <Stat label="Unhealthy" value={overview.counts.unhealthy_containers} tone="warn" />
                  </div>
                )}
              </section>
            );
          }
          if (w.id === "servers") {
            return empty ? (
              <div
                key="servers-empty"
                className="rounded-[var(--radius)] border border-dashed border-[var(--border-strong)] bg-[var(--bg-1)] px-8 py-14 text-center"
              >
                <h2 className="text-xl font-semibold">No servers connected</h2>
                <p className="mx-auto mt-2 max-w-md text-sm text-[var(--text-1)]">
                  Connect your first server to begin monitoring your infrastructure. Create a server record,
                  generate an enrollment token, then install the agent.
                </p>
                <Link
                  href="/servers"
                  className="mt-6 inline-flex rounded-md bg-[var(--accent)] px-4 py-2.5 text-sm font-medium text-white hover:brightness-110"
                >
                  + Add Server
                </Link>
              </div>
            ) : (
              <section key="servers">
                <div className="mb-3 flex items-center justify-between">
                  <h2 className="text-lg font-semibold">Servers</h2>
                  <Link href="/servers" className="text-sm text-[var(--accent)]">
                    View all
                  </Link>
                </div>
                <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
                  {servers.map((s) => {
                    const memPct = pct(s.metrics?.mem_used_bytes, s.metrics?.mem_total_bytes);
                    const diskPct = pct(s.metrics?.disk_used_bytes, s.metrics?.disk_total_bytes);
                    const cpu = s.metrics?.cpu_pct ?? null;
                    const fresh = freshnessLabel(s.metrics?.last_updated ?? s.last_metrics_at, s.status);
                    return (
                      <Link
                        key={s.id}
                        href={`/servers/${s.id}`}
                        className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-4 transition-colors hover:border-[var(--border-strong)]"
                      >
                        <div className="flex items-start justify-between gap-3">
                          <div>
                            <div className="font-medium">{s.name}</div>
                            <div className="mt-1 text-xs text-[var(--text-2)]">
                              {[s.os_name, s.os_version].filter(Boolean).join(" ") || "Awaiting agent"}
                              {s.docker_available ? " · Docker" : ""}
                            </div>
                          </div>
                          <div className="flex flex-col items-end gap-1.5">
                            <StatusPill state={s.health_state || s.status} />
                            {agentVersionOutdated(s.agent_version, currentAgentVersion) && (
                              <StatusPill state="outdated" />
                            )}
                          </div>
                        </div>
                        <div className="mt-4 space-y-2">
                          <ResourceBar label="CPU" value={cpu} />
                          <ResourceBar label="RAM" value={memPct} />
                          <ResourceBar label="DISK" value={diskPct} />
                        </div>
                        <div className="mt-3 flex justify-between gap-3 text-[11px] text-[var(--text-2)]">
                          <span>
                            ↓ {formatBps(s.metrics?.net_rx_bps)} · ↑ {formatBps(s.metrics?.net_tx_bps)}
                          </span>
                          <span>up {formatUptime(s.metrics?.uptime_seconds)}</span>
                        </div>
                        <div className="mt-2 flex justify-between text-xs text-[var(--text-2)]">
                          <span>{s.running_containers ?? 0} containers</span>
                          <span
                            style={{
                              color:
                                fresh.tone === "ok"
                                  ? "var(--ok)"
                                  : fresh.tone === "warn"
                                    ? "var(--warn)"
                                    : fresh.tone === "crit"
                                      ? "var(--crit)"
                                      : "var(--text-2)",
                            }}
                          >
                            {fresh.label}
                          </span>
                        </div>
                      </Link>
                    );
                  })}
                </div>
              </section>
            );
          }
          return null;
        })}
    </div>
  );
}

function normalizeLayout(raw: unknown[]): Widget[] {
  const allowed: WidgetID[] = ["health", "counts", "servers"];
  const out: Widget[] = [];
  const seen = new Set<string>();
  for (const item of raw) {
    if (!item || typeof item !== "object") continue;
    const rec = item as { id?: string; visible?: boolean };
    if (!allowed.includes(rec.id as WidgetID) || seen.has(rec.id!)) continue;
    seen.add(rec.id!);
    out.push({ id: rec.id as WidgetID, visible: rec.visible !== false });
  }
  for (const id of allowed) {
    if (!seen.has(id)) out.push({ id, visible: true });
  }
  return out;
}

function Stat({
  label,
  value,
  tone,
}: {
  label: string;
  value: number;
  tone?: "ok" | "warn" | "crit";
}) {
  const color =
    tone === "ok"
      ? "var(--ok)"
      : tone === "warn"
        ? "var(--warn)"
        : tone === "crit"
          ? "var(--crit)"
          : "var(--text-0)";
  return (
    <div className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-4">
      <div className="text-[11px] uppercase tracking-[0.12em] text-[var(--text-2)]">{label}</div>
      <div className="mt-2 font-[family-name:var(--font-mono-family)] text-2xl" style={{ color }}>
        {value}
      </div>
    </div>
  );
}

function ErrorPanel({ message }: { message: string }) {
  return (
    <div className="rounded-[var(--radius)] border border-[var(--crit)]/30 bg-[var(--bg-1)] p-6">
      <h2 className="text-lg font-semibold">Dashboard unavailable</h2>
      <p className="mt-2 text-sm text-[var(--text-1)]">{message}</p>
    </div>
  );
}

function SkeletonDashboard() {
  return (
    <div className="space-y-4">
      <div className="h-8 w-48 animate-pulse rounded bg-[var(--bg-2)]" />
      <div className="h-40 animate-pulse rounded-[var(--radius)] bg-[var(--bg-2)]" />
      <div className="grid gap-3 md:grid-cols-3">
        <div className="h-28 animate-pulse rounded-[var(--radius)] bg-[var(--bg-2)]" />
        <div className="h-28 animate-pulse rounded-[var(--radius)] bg-[var(--bg-2)]" />
        <div className="h-28 animate-pulse rounded-[var(--radius)] bg-[var(--bg-2)]" />
      </div>
    </div>
  );
}
