"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { api } from "@/lib/api";
import { useRealtime } from "@/lib/realtime";
import { StatusPill } from "@/components/status-pill";

type ComposeRow = {
  id: string;
  server_id: string;
  server_name: string;
  project_name: string;
  status: string;
  containers: number;
  updated_at: string;
};

const ACTIONS = [
  { id: "up", label: "Up", hint: "docker compose up -d (needs compose file labels) or start existing" },
  { id: "down", label: "Down", hint: "Stop and remove project containers (volumes kept)" },
  { id: "start", label: "Start", hint: "Start all project containers" },
  { id: "stop", label: "Stop", hint: "Stop all project containers" },
  { id: "restart", label: "Restart", hint: "Restart all project containers" },
  { id: "pull", label: "Pull", hint: "Pull project images" },
] as const;

type ActionId = (typeof ACTIONS)[number]["id"];

export default function ComposePage() {
  const [rows, setRows] = useState<ComposeRow[]>([]);
  const [q, setQ] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [msg, setMsg] = useState<string | null>(null);
  const [pending, setPending] = useState<{ id: string; action: ActionId } | null>(null);
  const [busy, setBusy] = useState(false);

  const load = useCallback(() => {
    api
      .compose()
      .then((d) => {
        setRows(d.data ?? []);
        setError(null);
      })
      .catch((e) => setError(e instanceof Error ? e.message : "Failed to load compose projects"));
  }, []);

  useEffect(() => {
    const boot = window.setTimeout(load, 0);
    const poll = window.setInterval(load, 10000);
    return () => {
      window.clearTimeout(boot);
      window.clearInterval(poll);
    };
  }, [load]);

  useRealtime((type) => {
    if (type === "docker.updated" || type === "servers.updated") load();
  });

  const filtered = useMemo(() => {
    const needle = q.trim().toLowerCase();
    if (!needle) return rows;
    return rows.filter(
      (r) =>
        r.project_name.toLowerCase().includes(needle) ||
        r.server_name.toLowerCase().includes(needle) ||
        r.status.toLowerCase().includes(needle),
    );
  }, [rows, q]);

  async function runAction() {
    if (!pending) return;
    setBusy(true);
    setMsg(null);
    setError(null);
    try {
      const res = await api.composeAction(pending.id, pending.action);
      if (res.accepted) {
        setMsg(res.message ?? "Action accepted; waiting on agent.");
      } else {
        setMsg(`Compose ${pending.action} completed for project.`);
      }
      setPending(null);
      load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Compose action failed");
    } finally {
      setBusy(false);
    }
  }

  const pendingRow = pending ? rows.find((r) => r.id === pending.id) : null;
  const pendingMeta = pending ? ACTIONS.find((a) => a.id === pending.action) : null;

  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <div>
        <div className="text-[11px] font-medium uppercase tracking-[0.18em] text-[var(--text-2)]">
          Docker
        </div>
        <h1 className="mt-1 text-3xl font-semibold tracking-tight">Compose</h1>
        <p className="mt-1 text-sm text-[var(--text-1)]">
          Discovered Compose projects on enrolled hosts. Actions run on the host agent with
          confirmation (admin/operator).
        </p>
      </div>

      <input
        value={q}
        onChange={(e) => setQ(e.target.value)}
        placeholder="Search projects…"
        className="w-full rounded-md border border-[var(--border)] bg-[var(--bg-2)] px-3 py-2 text-sm"
      />

      {error && (
        <div className="rounded-md border border-[var(--crit)]/40 bg-[var(--crit)]/10 px-3 py-2 text-sm text-[var(--crit)]">
          {error}
        </div>
      )}
      {msg && (
        <div className="rounded-md border border-[var(--ok)]/40 bg-[var(--ok)]/10 px-3 py-2 text-sm text-[var(--ok)]">
          {msg}
        </div>
      )}

      {pending && pendingRow && pendingMeta && (
        <div className="rounded-[var(--radius)] border border-[var(--warn)]/50 bg-[var(--warn)]/10 p-4">
          <div className="text-sm font-medium text-[var(--text-0)]">
            Confirm compose {pendingMeta.label.toLowerCase()}
          </div>
          <p className="mt-1 text-sm text-[var(--text-1)]">
            {pendingMeta.hint} for <strong>{pendingRow.project_name}</strong> on{" "}
            <strong>{pendingRow.server_name}</strong> ({pendingRow.containers} containers).
          </p>
          <div className="mt-3 flex gap-2">
            <button
              type="button"
              disabled={busy}
              onClick={() => void runAction()}
              className="rounded-md bg-[var(--accent)] px-3 py-1.5 text-sm text-white disabled:opacity-60"
            >
              {busy ? "Working…" : `Confirm ${pendingMeta.label}`}
            </button>
            <button
              type="button"
              disabled={busy}
              onClick={() => setPending(null)}
              className="rounded-md border border-[var(--border)] px-3 py-1.5 text-sm"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {filtered.length === 0 ? (
        <div className="rounded-[var(--radius)] border border-dashed border-[var(--border-strong)] px-5 py-10 text-center text-sm text-[var(--text-2)]">
          {rows.length === 0 ? "No Compose projects detected yet." : "No rows match this search."}
        </div>
      ) : (
        <div className="overflow-hidden rounded-[var(--radius)] border border-[var(--border)]">
          <table className="w-full text-left text-sm">
            <thead className="bg-[var(--bg-2)] text-[11px] uppercase tracking-[0.12em] text-[var(--text-2)]">
              <tr>
                <th className="px-4 py-3 font-medium">Project</th>
                <th className="px-4 py-3 font-medium">Status</th>
                <th className="px-4 py-3 font-medium">Server</th>
                <th className="px-4 py-3 font-medium">Containers</th>
                <th className="px-4 py-3 font-medium">Actions</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((r) => (
                <tr key={r.id} className="border-t border-[var(--border)] bg-[var(--bg-1)]">
                  <td className="px-4 py-3 font-medium">{r.project_name}</td>
                  <td className="px-4 py-3">
                    <StatusPill state={r.status} />
                  </td>
                  <td className="px-4 py-3 text-[var(--text-1)]">{r.server_name}</td>
                  <td className="px-4 py-3">{r.containers}</td>
                  <td className="px-4 py-3">
                    <div className="flex flex-wrap gap-1">
                      {ACTIONS.map((a) => (
                        <button
                          key={a.id}
                          type="button"
                          className="rounded border border-[var(--border)] px-2 py-0.5 text-[11px] hover:border-[var(--accent)] hover:text-[var(--accent)] disabled:opacity-50"
                          disabled={busy}
                          onClick={() => {
                            setMsg(null);
                            setPending({ id: r.id, action: a.id });
                          }}
                        >
                          {a.label}
                        </button>
                      ))}
                    </div>
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
