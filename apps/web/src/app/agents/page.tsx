"use client";

import { useEffect, useState } from "react";
import { API_URL } from "@/lib/api";
import { StatusPill } from "@/components/status-pill";

type Agent = {
  id: string;
  server_id: string;
  server_name: string;
  agent_version: string;
  os: string;
  arch: string;
  status: string;
  enrolled_at: string | null;
  last_heartbeat_at: string | null;
  last_latency_ms: number | null;
};

export default function AgentsPage() {
  const [rows, setRows] = useState<Agent[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const boot = window.setTimeout(() => {
      fetch(`${API_URL}/api/v1/agents`, { credentials: "include", cache: "no-store" })
        .then(async (r) => {
          if (!r.ok) throw new Error("Could not load agents (sign-in required).");
          return r.json();
        })
        .then((d) => setRows(d.data ?? []))
        .catch((e) => setError(e instanceof Error ? e.message : "Failed"));
    }, 0);
    return () => window.clearTimeout(boot);
  }, []);

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <div>
        <h1 className="text-3xl font-semibold tracking-tight">Agents</h1>
        <p className="mt-1 text-sm text-[var(--text-1)]">
          Agent health, versions, and last heartbeat. Updates are never automatic without configuration.
        </p>
      </div>
      {error && (
        <div className="rounded-md border border-[var(--crit)]/40 bg-[var(--crit)]/10 px-3 py-2 text-sm text-[var(--crit)]">
          {error}
        </div>
      )}
      {rows.length === 0 && !error ? (
        <div className="rounded-[var(--radius)] border border-dashed border-[var(--border-strong)] px-5 py-10 text-center text-sm text-[var(--text-2)]">
          No agents enrolled yet.
        </div>
      ) : (
        <div className="overflow-hidden rounded-[var(--radius)] border border-[var(--border)]">
          <table className="w-full text-left text-sm">
            <thead className="bg-[var(--bg-2)] text-[11px] uppercase tracking-[0.12em] text-[var(--text-2)]">
              <tr>
                <th className="px-4 py-3">Server</th>
                <th className="px-4 py-3">Version</th>
                <th className="px-4 py-3">Platform</th>
                <th className="px-4 py-3">Status</th>
                <th className="px-4 py-3">Last heartbeat</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((a) => (
                <tr key={a.id} className="border-t border-[var(--border)] bg-[var(--bg-1)]">
                  <td className="px-4 py-3">{a.server_name}</td>
                  <td className="px-4 py-3 font-[family-name:var(--font-mono-family)] text-xs">
                    {a.agent_version || "—"}
                  </td>
                  <td className="px-4 py-3 text-[var(--text-2)]">
                    {[a.os, a.arch].filter(Boolean).join(" / ") || "—"}
                  </td>
                  <td className="px-4 py-3">
                    <StatusPill state={a.status === "online" ? "online" : a.status === "offline" ? "offline" : "pending"} />
                  </td>
                  <td className="px-4 py-3 text-[var(--text-2)]">
                    {a.last_heartbeat_at ? new Date(a.last_heartbeat_at).toLocaleString() : "—"}
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
