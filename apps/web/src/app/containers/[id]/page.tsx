"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import { API_URL, apiFetch } from "@/lib/api";
import { StatusPill } from "@/components/status-pill";
import { formatBytes } from "@/lib/format";

type Container = {
  id: string;
  server_id: string;
  server_name: string;
  container_id: string;
  name: string;
  image_ref: string;
  image_id: string;
  state: string;
  health: string;
  restart_count: number;
  compose_project: string;
  compose_service: string;
  labels: Record<string, string>;
  ports: unknown;
  started_at: string | null;
  container_created_at: string | null;
  last_seen_at: string;
  cpu_pct: number | null;
  mem_used_bytes: number | null;
  mem_limit_bytes: number | null;
  metrics_at: string | null;
};

type EnvEntry = { key: string; value: string; sensitive: boolean; masked: boolean };

const ACTIONS = ["start", "stop", "restart", "pause", "unpause"] as const;

function levelClass(line: string): string {
  const u = line.toUpperCase();
  if (u.includes("ERROR") || u.includes("FATAL") || u.includes("CRITICAL")) return "text-[var(--crit)]";
  if (u.includes("WARN")) return "text-[var(--warn)]";
  if (u.includes("DEBUG")) return "text-[var(--text-2)]";
  return "text-[var(--text-1)]";
}

export default function ContainerDetailPage() {
  const { id } = useParams<{ id: string }>();
  const [c, setC] = useState<Container | null>(null);
  const [logs, setLogs] = useState<string[]>([]);
  const [logError, setLogError] = useState<string | null>(null);
  const [loadingLogs, setLoadingLogs] = useState(false);
  const [wrap, setWrap] = useState(true);
  const [filter, setFilter] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [pendingAction, setPendingAction] = useState<(typeof ACTIONS)[number] | null>(null);
  const [actionBusy, setActionBusy] = useState(false);
  const [actionMsg, setActionMsg] = useState<string | null>(null);
  const [env, setEnv] = useState<EnvEntry[] | null>(null);
  const [envRevealed, setEnvRevealed] = useState(false);
  const [envError, setEnvError] = useState<string | null>(null);
  const [envBusy, setEnvBusy] = useState(false);

  useEffect(() => {
    const boot = window.setTimeout(() => {
      fetch(`${API_URL}/api/v1/containers/${id}`, {
        credentials: "include",
        cache: "no-store",
      })
        .then(async (r) => {
          if (!r.ok) throw new Error("Container not found");
          return r.json();
        })
        .then(setC)
        .catch((e) => setError(e instanceof Error ? e.message : "Failed to load"));
    }, 0);
    return () => window.clearTimeout(boot);
  }, [id]);

  async function loadLogs() {
    setLoadingLogs(true);
    setLogError(null);
    try {
      const res = await fetch(`${API_URL}/api/v1/containers/${id}/logs?tail=300`, {
        credentials: "include",
        cache: "no-store",
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data?.error?.message ?? "Could not fetch logs");
      setLogs(data.lines ?? []);
    } catch (e) {
      setLogError(e instanceof Error ? e.message : "Log fetch failed");
    } finally {
      setLoadingLogs(false);
    }
  }

  async function runAction(action: (typeof ACTIONS)[number]) {
    setActionBusy(true);
    setActionMsg(null);
    try {
      const res = await apiFetch(`/api/v1/containers/${id}/actions/${action}`, {
        method: "POST",
        body: JSON.stringify({ confirm: true }),
      });
      const data = await res.json();
      if (!res.ok && res.status !== 202) {
        throw new Error(data?.error?.message ?? "Action failed");
      }
      setActionMsg(
        res.status === 202
          ? `Queued ${action}; waiting for agent…`
          : `${action} completed.`,
      );
      setPendingAction(null);
      const refreshed = await fetch(`${API_URL}/api/v1/containers/${id}`, {
        credentials: "include",
        cache: "no-store",
      });
      if (refreshed.ok) setC(await refreshed.json());
    } catch (e) {
      setActionMsg(e instanceof Error ? e.message : "Action failed");
    } finally {
      setActionBusy(false);
    }
  }

  async function loadEnv(reveal: boolean) {
    setEnvBusy(true);
    setEnvError(null);
    try {
      const q = reveal ? "?reveal=1" : "";
      const res = await fetch(`${API_URL}/api/v1/containers/${id}/env${q}`, {
        credentials: "include",
        cache: "no-store",
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data?.error?.message ?? "Could not load env");
      setEnv(data.env ?? []);
      setEnvRevealed(Boolean(data.revealed));
    } catch (e) {
      setEnvError(e instanceof Error ? e.message : "Env load failed");
    } finally {
      setEnvBusy(false);
    }
  }

  if (error) {
    return (
      <div className="rounded-[var(--radius)] border border-[var(--crit)]/30 bg-[var(--bg-1)] p-6">
        <h1 className="text-lg font-semibold">Container unavailable</h1>
        <p className="mt-2 text-sm text-[var(--text-1)]">{error}</p>
      </div>
    );
  }
  if (!c) return <div className="h-64 animate-pulse rounded-[var(--radius)] bg-[var(--bg-2)]" />;

  const filtered = logs.filter((l) => !filter || l.toLowerCase().includes(filter.toLowerCase()));

  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="text-[11px] font-medium uppercase tracking-[0.18em] text-[var(--text-2)]">
            Container
          </div>
          <h1 className="mt-1 text-3xl font-semibold tracking-tight">{c.name || c.container_id.slice(0, 12)}</h1>
          <p className="mt-1 text-sm text-[var(--text-1)]">
            <Link href={`/servers/${c.server_id}`} className="text-[var(--accent)]">
              {c.server_name}
            </Link>
            {" · "}
            {c.image_ref}
          </p>
        </div>
        <div className="flex gap-2">
          <StatusPill state={c.state === "running" ? "online" : c.state} />
          <StatusPill
            state={c.health === "unhealthy" ? "critical" : c.health === "healthy" ? "healthy" : "unknown"}
          />
        </div>
      </div>

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4 text-sm">
        <Stat label="CPU" value={c.cpu_pct == null ? "—" : `${c.cpu_pct.toFixed(1)}%`} />
        <Stat
          label="Memory"
          value={
            c.mem_used_bytes == null
              ? "—"
              : `${formatBytes(c.mem_used_bytes)} / ${formatBytes(c.mem_limit_bytes)}`
          }
        />
        <Stat label="Restarts" value={String(c.restart_count)} />
        <Stat label="Compose" value={c.compose_project || "—"} />
      </div>

      <section className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-4">
        <h2 className="text-lg font-semibold">Management</h2>
        <p className="mt-1 text-xs text-[var(--text-2)]">
          Actions run on the host via the agent. Confirmation is required; each action is audited.
        </p>
        <div className="mt-3 flex flex-wrap gap-2">
          {ACTIONS.map((a) => (
            <button
              key={a}
              type="button"
              className="rounded-md border border-[var(--border)] px-3 py-1.5 text-xs capitalize text-[var(--text-1)] hover:bg-[var(--bg-2)] disabled:opacity-50"
              disabled={actionBusy}
              onClick={() => setPendingAction(a)}
            >
              {a}
            </button>
          ))}
        </div>
        {actionMsg && <p className="mt-3 text-sm text-[var(--text-1)]">{actionMsg}</p>}
        {pendingAction && (
          <div className="mt-4 rounded-md border border-[var(--warn)]/40 bg-[var(--warn)]/10 p-3">
            <p className="text-sm text-[var(--text-0)]">
              Confirm <span className="font-medium capitalize">{pendingAction}</span> on{" "}
              <span className="font-[family-name:var(--font-mono-family)]">{c.name}</span>?
            </p>
            <div className="mt-3 flex gap-2">
              <button
                type="button"
                className="rounded-md bg-[var(--accent)] px-3 py-1.5 text-xs text-white disabled:opacity-60"
                disabled={actionBusy}
                onClick={() => void runAction(pendingAction)}
              >
                {actionBusy ? "Running…" : `Confirm ${pendingAction}`}
              </button>
              <button
                type="button"
                className="rounded-md border border-[var(--border)] px-3 py-1.5 text-xs"
                disabled={actionBusy}
                onClick={() => setPendingAction(null)}
              >
                Cancel
              </button>
            </div>
          </div>
        )}
      </section>

      <section className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-4">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 className="text-lg font-semibold">Environment</h2>
            <p className="text-xs text-[var(--text-2)]">
              Sensitive keys masked by default. Reveal is admin-only and audited.
            </p>
          </div>
          <div className="flex gap-2">
            <button
              type="button"
              className="rounded-md border border-[var(--border)] px-3 py-1 text-xs disabled:opacity-60"
              disabled={envBusy}
              onClick={() => void loadEnv(false)}
            >
              {envBusy ? "Loading…" : "Load env"}
            </button>
            <button
              type="button"
              className="rounded-md border border-[var(--warn)]/50 px-3 py-1 text-xs text-[var(--warn)] disabled:opacity-60"
              disabled={envBusy}
              onClick={() => {
                if (window.confirm("Reveal sensitive environment values? This is audited.")) {
                  void loadEnv(true);
                }
              }}
            >
              Reveal sensitive
            </button>
          </div>
        </div>
        {envError && (
          <div className="mb-3 rounded-md border border-[var(--crit)]/40 bg-[var(--crit)]/10 px-3 py-2 text-sm text-[var(--crit)]">
            {envError}
          </div>
        )}
        {env && (
          <div className="max-h-64 overflow-auto rounded-md border border-[var(--border)]">
            <table className="w-full text-left text-xs">
              <thead className="sticky top-0 bg-[var(--bg-2)] text-[var(--text-2)]">
                <tr>
                  <th className="px-3 py-2 font-medium">Key</th>
                  <th className="px-3 py-2 font-medium">Value{envRevealed ? " (revealed)" : ""}</th>
                </tr>
              </thead>
              <tbody>
                {env.map((e) => (
                  <tr key={e.key} className="border-t border-[var(--border)]">
                    <td className="px-3 py-1.5 font-[family-name:var(--font-mono-family)]">{e.key}</td>
                    <td className="px-3 py-1.5 font-[family-name:var(--font-mono-family)] text-[var(--text-1)]">
                      {e.value}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <section className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-4">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
          <h2 className="text-lg font-semibold">Logs</h2>
          <div className="flex flex-wrap gap-2">
            <input
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              placeholder="Filter…"
              className="rounded-md border border-[var(--border)] bg-[var(--bg-2)] px-2 py-1 text-xs"
            />
            <button
              type="button"
              className="rounded-md border border-[var(--border)] px-2 py-1 text-xs"
              onClick={() => setWrap((v) => !v)}
            >
              {wrap ? "Unwrap" : "Wrap"}
            </button>
            <button
              type="button"
              className="rounded-md bg-[var(--accent)] px-3 py-1 text-xs text-white disabled:opacity-60"
              disabled={loadingLogs}
              onClick={() => void loadLogs()}
            >
              {loadingLogs ? "Fetching…" : "Fetch logs"}
            </button>
          </div>
        </div>
        {logError && (
          <div className="mb-3 rounded-md border border-[var(--crit)]/40 bg-[var(--crit)]/10 px-3 py-2 text-sm text-[var(--crit)]">
            {logError}
          </div>
        )}
        <pre
          className={`max-h-[480px] overflow-auto rounded-md bg-[var(--bg-0)] p-3 font-[family-name:var(--font-mono-family)] text-xs ${
            wrap ? "whitespace-pre-wrap break-words" : "whitespace-pre"
          }`}
        >
          {filtered.length === 0
            ? "No logs loaded yet. Fetch logs from the agent (read-only Docker API on the host)."
            : filtered.map((line, i) => (
                <div key={i} className={levelClass(line)}>
                  {line}
                </div>
              ))}
        </pre>
        <p className="mt-2 text-[11px] text-[var(--text-2)]">
          Logs are rendered as plain text only — HTML/JS from containers cannot execute in the UI.
        </p>
      </section>
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-3">
      <div className="text-[11px] uppercase tracking-[0.12em] text-[var(--text-2)]">{label}</div>
      <div className="mt-1 font-[family-name:var(--font-mono-family)] text-[var(--text-0)]">{value}</div>
    </div>
  );
}
