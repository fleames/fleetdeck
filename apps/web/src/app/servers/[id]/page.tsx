"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { MetricsChart } from "@/components/metrics-chart";
import { API_URL, apiFetch } from "@/lib/api";
import { StatusPill, agentVersionOutdated, agentSupportsPanelUpdate, AGENT_MANUAL_UPGRADE_CMD } from "@/components/status-pill";
import { ResourceBar } from "@/components/resource-bar";
import { formatBps, formatBytes, formatUptime, freshnessLabel, pct } from "@/lib/format";
import { historyGapMs, withTimeGaps } from "@/lib/chart-series";
import { ErrorBanner, EmptyBlock, LoadingBlock } from "@/components/page-state";

type ServerDetail = {
  id: string;
  name: string;
  hostname: string;
  primary_address: string;
  os_name: string;
  os_version: string;
  arch: string;
  status: string;
  health_state: string;
  docker_available: boolean;
  last_seen_at: string | null;
  last_metrics_at?: string | null;
  agent_version?: string | null;
  current_agent_version?: string | null;
  running_containers?: number;
  metrics?: {
    cpu_pct: number | null;
    mem_used_bytes: number | null;
    mem_total_bytes: number | null;
    disk_used_bytes: number | null;
    disk_total_bytes: number | null;
    net_rx_bps: number | null;
    net_tx_bps: number | null;
    uptime_seconds: number | null;
    last_updated: string | null;
  } | null;
};

type HistoryPoint = {
  ts: string;
  cpu_pct: number | null;
  mem_used_bytes: number | null;
  disk_used_bytes: number | null;
  disk_total_bytes: number | null;
  net_rx_bps: number | null;
  net_tx_bps: number | null;
};

const ranges = ["15m", "1h", "6h", "24h", "7d", "30d"] as const;

function agentLooksOnline(s: ServerDetail): boolean {
  if (s.status !== "online") return false;
  if (!s.last_seen_at) return false;
  return Date.now() - new Date(s.last_seen_at).getTime() < 2 * 60 * 1000;
}

export default function ServerDetailPage() {
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const id = params.id;
  const [server, setServer] = useState<ServerDetail | null>(null);
  const [history, setHistory] = useState<HistoryPoint[]>([]);
  const [historyMeta, setHistoryMeta] = useState<{ source?: string; truncated?: boolean }>({});
  const [range, setRange] = useState<(typeof ranges)[number]>("1h");
  const [tab, setTab] = useState<"overview" | "metrics" | "docker" | "events">("overview");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [confirmRemove, setConfirmRemove] = useState(false);
  const [removeBusy, setRemoveBusy] = useState(false);
  const [removeMsg, setRemoveMsg] = useState<string | null>(null);
  const [confirmUpdate, setConfirmUpdate] = useState(false);
  const [updateBusy, setUpdateBusy] = useState(false);
  const [updateMsg, setUpdateMsg] = useState<string | null>(null);
  const [upgradeCopied, setUpgradeCopied] = useState(false);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      try {
        const [sRes, hRes] = await Promise.all([
          fetch(`${API_URL}/api/v1/servers/${id}`, { credentials: "include", cache: "no-store" }),
          fetch(`${API_URL}/api/v1/servers/${id}/metrics/history?range=${range}`, {
            credentials: "include",
            cache: "no-store",
          }),
        ]);
        if (!sRes.ok) throw new Error("Server not found");
        const s = await sRes.json();
        const h = await hRes.json();
        if (cancelled) return;
        setServer(s);
        setHistory(h.data ?? []);
        setHistoryMeta({ source: h.source, truncated: h.truncated });
        setError(null);
      } catch (e) {
        if (!cancelled) setError(e instanceof Error ? e.message : "Failed to load");
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    const boot = window.setTimeout(() => void load(), 0);
    const poll = window.setInterval(() => void load(), 8000);
    return () => {
      cancelled = true;
      window.clearTimeout(boot);
      window.clearInterval(poll);
    };
  }, [id, range]);

  const chartOption = useMemo(() => {
    const gapMs = historyGapMs(historyMeta.source, range);
    const gapped = withTimeGaps(history, gapMs);
    const times = gapped.map((p) => (p ? new Date(p.ts).toLocaleTimeString() : ""));
    const cpu = gapped.map((p) => (p == null ? null : p.cpu_pct));
    const mem = gapped.map((p) => (p == null ? null : p.mem_used_bytes));
    const rx = gapped.map((p) => (p == null ? null : p.net_rx_bps));
    const tx = gapped.map((p) => (p == null ? null : p.net_tx_bps));
    return {
      backgroundColor: "transparent",
      textStyle: { color: "#8b97a8" },
      tooltip: { trigger: "axis" },
      legend: { data: ["CPU %", "RAM used", "Net ↓", "Net ↑"], textStyle: { color: "#c3ccd8" } },
      grid: { left: 48, right: 24, top: 40, bottom: 32 },
      xAxis: { type: "category", data: times, axisLabel: { color: "#8b97a8" } },
      yAxis: [
        { type: "value", name: "%", axisLabel: { color: "#8b97a8" }, splitLine: { lineStyle: { color: "#2a3340" } } },
        { type: "value", name: "bytes", axisLabel: { color: "#8b97a8" }, splitLine: { show: false } },
      ],
      series: [
        {
          name: "CPU %",
          type: "line",
          showSymbol: false,
          connectNulls: false,
          data: cpu,
          lineStyle: { color: "#3d9cf0", width: 2 },
          areaStyle: { color: "rgba(61,156,240,0.12)" },
        },
        {
          name: "RAM used",
          type: "line",
          yAxisIndex: 1,
          showSymbol: false,
          connectNulls: false,
          data: mem,
          lineStyle: { color: "#3ecf8e", width: 2 },
        },
        {
          name: "Net ↓",
          type: "line",
          yAxisIndex: 1,
          showSymbol: false,
          connectNulls: false,
          data: rx,
          lineStyle: { color: "#e6b84d", width: 1.5 },
        },
        {
          name: "Net ↑",
          type: "line",
          yAxisIndex: 1,
          showSymbol: false,
          connectNulls: false,
          data: tx,
          lineStyle: { color: "#f07178", width: 1.5 },
        },
      ],
    };
  }, [history, historyMeta.source, range]);

  async function removeServer(force: boolean) {
    if (!server) return;
    setRemoveBusy(true);
    setRemoveMsg(null);
    try {
      const res = await apiFetch(`/api/v1/servers/${server.id}/remove`, {
        method: "POST",
        body: JSON.stringify({ confirm: true, force }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        setRemoveMsg(data?.error?.message ?? "Could not remove server");
        return;
      }
      router.push("/servers");
    } catch (e) {
      setRemoveMsg(e instanceof Error ? e.message : "Could not remove server");
    } finally {
      setRemoveBusy(false);
    }
  }

  async function updateAgent() {
    if (!server) return;
    setUpdateBusy(true);
    setUpdateMsg(null);
    try {
      const res = await apiFetch(`/api/v1/servers/${server.id}/update-agent`, {
        method: "POST",
        body: JSON.stringify({ confirm: true }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        const msg = data?.error?.message ?? "Could not update agent";
        if (/unsupported|unknown command|agent\.update/i.test(msg) || !agentSupportsPanelUpdate(server.agent_version)) {
          setUpdateMsg(
            `${msg} This agent is too old for panel update. On the VPS run:\n${AGENT_MANUAL_UPGRADE_CMD}`,
          );
          return;
        }
        setUpdateMsg(msg);
        return;
      }
      setConfirmUpdate(false);
      setUpdateMsg(
        data.message ?? "Update staged; agent is restarting. Version refreshes on next heartbeat.",
      );
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Could not update agent";
      if (/unsupported|unknown command|agent\.update/i.test(msg)) {
        setUpdateMsg(
          `${msg} This agent is too old for panel update. On the VPS run:\n${AGENT_MANUAL_UPGRADE_CMD}`,
        );
      } else {
        setUpdateMsg(msg);
      }
    } finally {
      setUpdateBusy(false);
    }
  }

  async function copyUpgradeCommand() {
    await navigator.clipboard.writeText(AGENT_MANUAL_UPGRADE_CMD);
    setUpgradeCopied(true);
    window.setTimeout(() => setUpgradeCopied(false), 2000);
  }

  if (error) {
    return (
      <div className="mx-auto max-w-6xl space-y-4">
        <h1 className="text-lg font-semibold">Server unavailable</h1>
        <ErrorBanner message={error} />
      </div>
    );
  }

  if (loading || !server) {
    return <LoadingBlock label="Loading server" />;
  }

  const memPct = pct(server.metrics?.mem_used_bytes, server.metrics?.mem_total_bytes);
  const diskPct = pct(server.metrics?.disk_used_bytes, server.metrics?.disk_total_bytes);
  const fresh = freshnessLabel(server.metrics?.last_updated ?? server.last_metrics_at, server.status);
  const online = agentLooksOnline(server);

  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <div className="text-[11px] font-medium uppercase tracking-[0.18em] text-[var(--text-2)]">Server</div>
          <h1 className="mt-1 text-3xl font-semibold tracking-tight">{server.name}</h1>
          <p className="mt-1 text-sm text-[var(--text-1)]">
            {[server.hostname, server.os_name, server.os_version, server.arch].filter(Boolean).join(" · ") ||
              "Awaiting agent inventory"}
          </p>
        </div>
        <div className="flex items-center gap-3">
          <StatusPill state={server.health_state || server.status} />
          {agentVersionOutdated(server.agent_version, server.current_agent_version) && (
            <button
              type="button"
              className="inline-flex"
              title={
                server.current_agent_version
                  ? `Expected ${server.current_agent_version}. Open Update agent.`
                  : "Open Update agent"
              }
              onClick={() => {
                setConfirmUpdate(true);
                setUpdateMsg(null);
                setConfirmRemove(false);
              }}
            >
              <StatusPill state="outdated" />
            </button>
          )}
          <span className="text-xs text-[var(--text-2)]">{fresh.label}</span>
          {server.agent_version && (
            <button
              type="button"
              className="rounded-md border border-[var(--border)] px-3 py-1.5 text-xs text-[var(--text-1)] hover:bg-[var(--bg-2)]"
              onClick={() => {
                setConfirmUpdate(true);
                setUpdateMsg(null);
                setConfirmRemove(false);
              }}
            >
              Update agent
            </button>
          )}
          <button
            type="button"
            className="rounded-md border border-[var(--crit)]/40 px-3 py-1.5 text-xs text-[var(--crit)] hover:bg-[var(--crit)]/10"
            onClick={() => {
              setConfirmRemove(true);
              setRemoveMsg(null);
              setConfirmUpdate(false);
            }}
          >
            Remove
          </button>
        </div>
      </div>

      {confirmUpdate && (
        <section className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-4">
          <h2 className="text-sm font-medium">Update agent on {server.name}?</h2>
          {online ? (
            agentSupportsPanelUpdate(server.agent_version) ? (
              <p className="mt-2 text-sm text-[var(--text-1)]">
                Current version {server.agent_version || "—"}. Pulls CDN channel binary, verifies SHA256 when
                present, replaces binary, restarts systemd. Credentials are preserved.
              </p>
            ) : (
              <div className="mt-2 space-y-2 text-sm text-[var(--text-1)]">
                <p>
                  Agent {server.agent_version || "unknown"} does not support panel Update (needs 0.4.2+).
                  Run this on the VPS — credentials are kept, no new token:
                </p>
                <pre className="overflow-x-auto rounded-md border border-[var(--border)] bg-[var(--bg-2)] px-3 py-2 font-[family-name:var(--font-mono-family)] text-xs">
                  {AGENT_MANUAL_UPGRADE_CMD}
                </pre>
                <button
                  type="button"
                  className="rounded-md border border-[var(--border)] px-3 py-1.5 text-xs"
                  onClick={() => void copyUpgradeCommand()}
                >
                  {upgradeCopied ? "Copied" : "Copy command"}
                </button>
              </div>
            )
          ) : (
            <p className="mt-2 text-sm text-[var(--text-1)]">
              Agent offline — wait until online, then retry. Or run the manual upgrade one-liner on the
              host if the agent is too old for panel update.
            </p>
          )}
          {updateMsg && <pre className="mt-2 whitespace-pre-wrap text-sm text-[var(--crit)]">{updateMsg}</pre>}
          <div className="mt-3 flex flex-wrap gap-2">
            {online && agentSupportsPanelUpdate(server.agent_version) && (
              <button
                type="button"
                disabled={updateBusy}
                className="rounded-md bg-[var(--accent)] px-3 py-1.5 text-xs text-white disabled:opacity-60"
                onClick={() => void updateAgent()}
              >
                {updateBusy ? "Updating…" : "Update agent"}
              </button>
            )}
            {online && !agentSupportsPanelUpdate(server.agent_version) && (
              <button
                type="button"
                disabled={updateBusy}
                className="rounded-md border border-[var(--border)] px-3 py-1.5 text-xs disabled:opacity-60"
                onClick={() => void updateAgent()}
              >
                {updateBusy ? "Trying…" : "Try panel update anyway"}
              </button>
            )}
            <button
              type="button"
              disabled={updateBusy}
              className="rounded-md border border-[var(--border)] px-3 py-1.5 text-xs"
              onClick={() => setConfirmUpdate(false)}
            >
              {online ? "Cancel" : "Dismiss"}
            </button>
          </div>
        </section>
      )}

      {updateMsg && !confirmUpdate && (
        <div className="whitespace-pre-wrap rounded-md border border-[var(--border)] bg-[var(--bg-1)] px-3 py-2 text-sm text-[var(--text-1)]">
          {updateMsg}
        </div>
      )}

      {confirmRemove && (
        <section className="rounded-[var(--radius)] border border-[var(--crit)]/40 bg-[var(--crit)]/10 p-4">
          <h2 className="text-sm font-medium text-[var(--crit)]">Remove {server.name}?</h2>
          {online ? (
            <p className="mt-2 text-sm text-[var(--text-1)]">
              Agent is online — uninstall on the VPS (systemd stop/disable, binary, state), then delete panel
              records.
            </p>
          ) : server.status === "pending" ? (
            <p className="mt-2 text-sm text-[var(--text-1)]">Never enrolled — deletes panel records only.</p>
          ) : (
            <p className="mt-2 text-sm text-[var(--text-1)]">
              Agent offline — force remove deletes panel records only. Manual uninstall:{" "}
              <code className="text-xs">sudo fleetdeck-agent -uninstall</code>
            </p>
          )}
          {removeMsg && <p className="mt-2 text-sm text-[var(--crit)]">{removeMsg}</p>}
          <div className="mt-3 flex flex-wrap gap-2">
            {(online || server.status === "pending") && (
              <button
                type="button"
                disabled={removeBusy}
                className="rounded-md bg-[var(--crit)] px-3 py-1.5 text-xs text-white disabled:opacity-60"
                onClick={() => void removeServer(false)}
              >
                {removeBusy ? "Removing…" : online ? "Uninstall & remove" : "Remove"}
              </button>
            )}
            <button
              type="button"
              disabled={removeBusy}
              className="rounded-md border border-[var(--crit)]/50 px-3 py-1.5 text-xs text-[var(--crit)] disabled:opacity-60"
              onClick={() => void removeServer(true)}
            >
              Force remove from panel
            </button>
            <button
              type="button"
              disabled={removeBusy}
              className="rounded-md border border-[var(--border)] px-3 py-1.5 text-xs"
              onClick={() => setConfirmRemove(false)}
            >
              Cancel
            </button>
          </div>
        </section>
      )}

      <div className="flex flex-wrap gap-2">
        {(["overview", "metrics", "docker", "events"] as const).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`rounded-md px-3 py-1.5 text-sm capitalize ${
              tab === t ? "bg-[var(--bg-3)] text-[var(--text-0)]" : "text-[var(--text-1)] hover:bg-[var(--bg-2)]"
            }`}
          >
            {t}
          </button>
        ))}
      </div>

      {tab === "overview" && (
        <div className="grid gap-4 lg:grid-cols-[1fr_1fr]">
          <div className="space-y-3 rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-5">
            <ResourceBar label="CPU" value={server.metrics?.cpu_pct ?? null} />
            <ResourceBar label="RAM" value={memPct} />
            <ResourceBar label="DISK" value={diskPct} />
            <div className="grid grid-cols-2 gap-3 pt-2 text-sm text-[var(--text-1)]">
              <div>Network ↓ {formatBps(server.metrics?.net_rx_bps)}</div>
              <div>Network ↑ {formatBps(server.metrics?.net_tx_bps)}</div>
              <div>Uptime {formatUptime(server.metrics?.uptime_seconds)}</div>
              <div>Containers {server.running_containers ?? 0}</div>
              <div>RAM {formatBytes(server.metrics?.mem_used_bytes)} / {formatBytes(server.metrics?.mem_total_bytes)}</div>
              <div>Disk {formatBytes(server.metrics?.disk_used_bytes)} / {formatBytes(server.metrics?.disk_total_bytes)}</div>
              <div>Address {server.primary_address || "—"}</div>
              <div className="flex flex-wrap items-center gap-2">
                <span>Agent {server.agent_version || "—"}</span>
                {agentVersionOutdated(server.agent_version, server.current_agent_version) && (
                  <StatusPill state="outdated" />
                )}
              </div>
            </div>
          </div>
          <div className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-3">
            {(historyMeta.source || historyMeta.truncated) && (
              <div className="mb-2 text-[11px] text-[var(--text-2)]">
                {historyMeta.source ? `Source: ${historyMeta.source}` : null}
                {historyMeta.truncated ? " · range capped to retention" : null}
                {" · gaps break the line (no invented samples)"}
              </div>
            )}
            {history.length === 0 ? (
              <EmptyBlock>Metrics unavailable — no history for this range yet.</EmptyBlock>
            ) : (
              <MetricsChart option={chartOption} height={280} />
            )}
          </div>
        </div>
      )}

      {tab === "metrics" && (
        <div className="space-y-4">
          <div className="flex flex-wrap gap-2">
            {ranges.map((r) => (
              <button
                key={r}
                onClick={() => setRange(r)}
                className={`rounded-md px-3 py-1.5 text-xs font-[family-name:var(--font-mono-family)] ${
                  range === r ? "bg-[var(--accent)] text-white" : "border border-[var(--border)] text-[var(--text-1)]"
                }`}
              >
                {r}
              </button>
            ))}
          </div>
          <div className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-3">
            {(historyMeta.source || historyMeta.truncated) && (
              <div className="mb-2 text-[11px] text-[var(--text-2)]">
                {historyMeta.source ? `Source: ${historyMeta.source}` : null}
                {historyMeta.truncated ? " · range capped to retention" : null}
                {" · gaps break the line (no invented samples)"}
              </div>
            )}
            {history.length === 0 ? (
              <EmptyBlock>No samples in this range.</EmptyBlock>
            ) : (
              <MetricsChart option={chartOption} height={360} />
            )}
          </div>
        </div>
      )}

      {tab === "docker" && (
        <div className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-5 text-sm text-[var(--text-1)]">
          Docker available: <strong>{server.docker_available ? "yes" : "no"}</strong>
          <div className="mt-2">Running containers on this host: {server.running_containers ?? 0}</div>
          <div className="mt-4 text-[var(--text-2)]">
            Full container/image/volume drill-down for this server uses the global Docker explorers filtered by server.
          </div>
          <Link
            href={`/containers?server=${server.id}`}
            className="mt-3 inline-block text-sm text-[var(--accent)] hover:underline"
          >
            Open containers for this server
          </Link>
        </div>
      )}

      {tab === "events" && (
        <div className="rounded-[var(--radius)] border border-dashed border-[var(--border-strong)] px-5 py-10 text-center text-sm text-[var(--text-2)]">
          Server-scoped event timeline uses the global Events feed (filter by server coming next).
        </div>
      )}
    </div>
  );
}
