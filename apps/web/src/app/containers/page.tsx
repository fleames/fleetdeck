"use client";

import { Suspense, useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { api } from "@/lib/api";
import { StatusPill } from "@/components/status-pill";
import { useRealtime } from "@/lib/realtime";
import { EmptyBlock, ErrorBanner, LoadingBlock } from "@/components/page-state";

type Container = {
  id: string;
  server_id: string;
  server_name: string;
  container_id: string;
  name: string;
  image_ref: string;
  state: string;
  health: string;
  restart_count: number;
  compose_project: string;
  last_seen_at: string;
};

type Filter = "all" | "running" | "stopped" | "unhealthy";

/** Host containers safe to docker-rm (matches API clear-stale). */
const STALE_STATES = new Set(["exited", "dead", "created"]);

function isStaleState(state: string): boolean {
  return STALE_STATES.has(state.toLowerCase());
}

function parseFilter(raw: string | null): Filter {
  if (raw === "running" || raw === "stopped" || raw === "unhealthy") return raw;
  return "all";
}

export default function ContainersPage() {
  return (
    <Suspense fallback={<LoadingBlock label="Loading containers" />}>
      <ContainersPageInner />
    </Suspense>
  );
}

function ContainersPageInner() {
  const searchParams = useSearchParams();
  const router = useRouter();
  const [rows, setRows] = useState<Container[]>([]);
  const [q, setQ] = useState("");
  const filter = parseFilter(searchParams.get("filter"));
  const serverId = searchParams.get("server") || searchParams.get("server_id") || "";
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [confirmClear, setConfirmClear] = useState(false);
  const [clearBusy, setClearBusy] = useState(false);
  const [clearMsg, setClearMsg] = useState<string | null>(null);

  const load = useCallback((opts?: { soft?: boolean }) => {
    api
      .containers()
      .then((res) => {
        setRows(res.data);
        setError(null);
      })
      .catch((e) => {
        if (!opts?.soft) setError(e instanceof Error ? e.message : "Failed to load");
      })
      .finally(() => {
        if (!opts?.soft) setLoading(false);
      });
  }, []);

  useEffect(() => {
    const boot = window.setTimeout(() => load(), 0);
    const poll = window.setInterval(() => load({ soft: true }), 10000);
    return () => {
      window.clearTimeout(boot);
      window.clearInterval(poll);
    };
  }, [load]);

  useRealtime((type) => {
    if (type === "docker.updated" || type === "servers.updated") load({ soft: true });
  });

  function setFilter(next: Filter) {
    const params = new URLSearchParams(searchParams.toString());
    if (next === "all") params.delete("filter");
    else params.set("filter", next);
    const qs = params.toString();
    router.replace(qs ? `/containers?${qs}` : "/containers");
  }

  const scopedRows = useMemo(() => {
    if (!serverId) return rows;
    return rows.filter((c) => c.server_id === serverId);
  }, [rows, serverId]);

  const filtered = useMemo(() => {
    const needle = q.trim().toLowerCase();
    return scopedRows.filter((c) => {
      if (filter === "running" && c.state !== "running") return false;
      if (filter === "stopped" && c.state === "running") return false;
      if (filter === "unhealthy" && c.health !== "unhealthy") return false;
      if (!needle) return true;
      return (
        c.name.toLowerCase().includes(needle) ||
        c.image_ref.toLowerCase().includes(needle) ||
        c.server_name.toLowerCase().includes(needle) ||
        c.compose_project.toLowerCase().includes(needle)
      );
    });
  }, [scopedRows, q, filter]);

  const staleRows = useMemo(
    () => scopedRows.filter((c) => isStaleState(c.state)),
    [scopedRows],
  );
  const serverLabel = staleRows[0]?.server_name || scopedRows[0]?.server_name || null;

  async function clearStale() {
    setClearBusy(true);
    setClearMsg(null);
    try {
      const res = await api.clearStaleContainers(serverId ? { serverId } : undefined);
      setClearMsg(
        res.queued > 0
          ? `Queued removal of ${res.queued} stale container${res.queued === 1 ? "" : "s"}.`
          : res.message || "No stale containers to remove.",
      );
      setConfirmClear(false);
      load({ soft: true });
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Clear stale failed";
      setClearMsg(
        /unsupported|unknown command|container\.remove/i.test(msg)
          ? `${msg} — rebuild/upgrade the agent so it supports container.remove.`
          : msg,
      );
    } finally {
      setClearBusy(false);
    }
  }

  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="text-[11px] font-medium uppercase tracking-[0.18em] text-[var(--text-2)]">
            Explorer
          </div>
          <h1 className="mt-1 text-3xl font-semibold tracking-tight">Containers</h1>
          <p className="mt-1 text-sm text-[var(--text-1)]">
            Live inventory from agents. Search is instant over the loaded set.
            {serverId && serverLabel ? (
              <>
                {" "}
                Scoped to{" "}
                <Link href={`/servers/${serverId}`} className="text-[var(--accent)] hover:underline">
                  {serverLabel}
                </Link>
                .
              </>
            ) : null}
          </p>
        </div>
        <button
          type="button"
          className="rounded-md border border-[var(--crit)]/40 px-3 py-1.5 text-xs text-[var(--crit)] hover:bg-[var(--crit)]/10 disabled:opacity-50"
          disabled={clearBusy || staleRows.length === 0}
          onClick={() => {
            setClearMsg(null);
            setConfirmClear(true);
          }}
        >
          Clear stale{staleRows.length > 0 ? ` (${staleRows.length})` : ""}
        </button>
      </div>

      {confirmClear && (
        <section className="rounded-[var(--radius)] border border-[var(--crit)]/40 bg-[var(--crit)]/10 p-4">
          <h2 className="text-sm font-medium text-[var(--crit)]">
            Clear {staleRows.length} stale container{staleRows.length === 1 ? "" : "s"}?
          </h2>
          <p className="mt-2 text-sm text-[var(--text-1)]">
            Removes <span className="font-medium">exited</span>, <span className="font-medium">dead</span>, and{" "}
            <span className="font-medium">created</span> containers via the agent
            {serverId ? " on this server" : " across the fleet"} (same as{" "}
            <code className="text-xs">docker rm</code>). Running, paused, and restarting containers are not
            touched. Requires admin or operator.
          </p>
          {staleRows.length > 0 && (
            <ul className="mt-3 max-h-40 overflow-auto text-xs text-[var(--text-2)]">
              {staleRows.slice(0, 40).map((c) => (
                <li key={c.id} className="font-[family-name:var(--font-mono-family)]">
                  {c.name || c.container_id.slice(0, 12)}
                  {!serverId ? ` @ ${c.server_name}` : ""} · {c.state}
                </li>
              ))}
              {staleRows.length > 40 && <li>…and {staleRows.length - 40} more</li>}
            </ul>
          )}
          <div className="mt-3 flex flex-wrap gap-2">
            <button
              type="button"
              disabled={clearBusy || staleRows.length === 0}
              className="rounded-md bg-[var(--crit)] px-3 py-1.5 text-xs text-white disabled:opacity-60"
              onClick={() => void clearStale()}
            >
              {clearBusy ? "Queuing…" : "Confirm clear stale"}
            </button>
            <button
              type="button"
              disabled={clearBusy}
              className="rounded-md border border-[var(--border)] px-3 py-1.5 text-xs"
              onClick={() => setConfirmClear(false)}
            >
              Cancel
            </button>
          </div>
        </section>
      )}

      {clearMsg && (
        <p className="text-sm text-[var(--text-1)]" role="status">
          {clearMsg}
        </p>
      )}

      <div className="flex flex-col gap-3 sm:flex-row">
        <input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="Search name, image, server, compose…"
          className="min-w-0 flex-1 rounded-md border border-[var(--border)] bg-[var(--bg-2)] px-3 py-2 text-sm"
        />
        <select
          value={filter}
          onChange={(e) => setFilter(e.target.value as Filter)}
          className="rounded-md border border-[var(--border)] bg-[var(--bg-2)] px-3 py-2 text-sm"
        >
          <option value="all">All</option>
          <option value="running">Running</option>
          <option value="stopped">Stopped</option>
          <option value="unhealthy">Unhealthy</option>
        </select>
      </div>

      {error && <ErrorBanner message={error} />}

      {loading && rows.length === 0 && !error ? (
        <LoadingBlock label="Loading containers" />
      ) : filtered.length === 0 ? (
        <EmptyBlock>
          {scopedRows.length === 0 ? "No containers reported yet." : "No containers match this filter."}
        </EmptyBlock>
      ) : (
        <div className="overflow-hidden rounded-[var(--radius)] border border-[var(--border)]">
          <table className="w-full text-left text-sm">
            <thead className="bg-[var(--bg-2)] text-[11px] uppercase tracking-[0.12em] text-[var(--text-2)]">
              <tr>
                <th className="px-4 py-3 font-medium">Container</th>
                <th className="px-4 py-3 font-medium">Server</th>
                <th className="px-4 py-3 font-medium">Image</th>
                <th className="px-4 py-3 font-medium">State</th>
                <th className="px-4 py-3 font-medium">Health</th>
                <th className="px-4 py-3 font-medium">Restarts</th>
              </tr>
            </thead>
            <tbody>
              {filtered.slice(0, 200).map((c) => (
                <tr key={c.id} className="border-t border-[var(--border)] bg-[var(--bg-1)]">
                  <td className="px-4 py-3">
                    <Link href={`/containers/${c.id}`} className="font-medium text-[var(--accent)] hover:underline">
                      {c.name || c.container_id.slice(0, 12)}
                    </Link>
                    {c.compose_project && (
                      <div className="text-xs text-[var(--text-2)]">{c.compose_project}</div>
                    )}
                  </td>
                  <td className="px-4 py-3 text-[var(--text-1)]">{c.server_name}</td>
                  <td className="max-w-[220px] truncate px-4 py-3 font-[family-name:var(--font-mono-family)] text-xs text-[var(--text-2)]">
                    {c.image_ref}
                  </td>
                  <td className="px-4 py-3">
                    <StatusPill state={c.state === "running" ? "online" : c.state} />
                  </td>
                  <td className="px-4 py-3">
                    <StatusPill
                      state={
                        c.health === "unhealthy"
                          ? "critical"
                          : c.health === "healthy"
                            ? "healthy"
                            : "unknown"
                      }
                    />
                  </td>
                  <td className="px-4 py-3 font-[family-name:var(--font-mono-family)] text-[var(--text-2)]">
                    {c.restart_count}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
