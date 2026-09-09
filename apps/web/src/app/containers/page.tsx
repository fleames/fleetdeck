"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { api } from "@/lib/api";
import { StatusPill } from "@/components/status-pill";
import { useRealtime } from "@/lib/realtime";

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

export default function ContainersPage() {
  const [rows, setRows] = useState<Container[]>([]);
  const [q, setQ] = useState("");
  const [filter, setFilter] = useState<"all" | "running" | "stopped" | "unhealthy">("all");
  const [error, setError] = useState<string | null>(null);

  const load = useCallback((opts?: { soft?: boolean }) => {
    api
      .containers()
      .then((res) => setRows(res.data))
      .catch((e) => {
        if (!opts?.soft) setError(e instanceof Error ? e.message : "Failed to load");
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

  const filtered = useMemo(() => {
    const needle = q.trim().toLowerCase();
    return rows.filter((c) => {
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
  }, [rows, q, filter]);

  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <div>
        <div className="text-[11px] font-medium uppercase tracking-[0.18em] text-[var(--text-2)]">
          Explorer
        </div>
        <h1 className="mt-1 text-3xl font-semibold tracking-tight">Containers</h1>
        <p className="mt-1 text-sm text-[var(--text-1)]">
          Live inventory from agents. Search is instant over the loaded set.
        </p>
      </div>

      <div className="flex flex-col gap-3 sm:flex-row">
        <input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="Search name, image, server, compose…"
          className="min-w-0 flex-1 rounded-md border border-[var(--border)] bg-[var(--bg-2)] px-3 py-2 text-sm"
        />
        <select
          value={filter}
          onChange={(e) => setFilter(e.target.value as typeof filter)}
          className="rounded-md border border-[var(--border)] bg-[var(--bg-2)] px-3 py-2 text-sm"
        >
          <option value="all">All</option>
          <option value="running">Running</option>
          <option value="stopped">Stopped</option>
          <option value="unhealthy">Unhealthy</option>
        </select>
      </div>

      {error && (
        <div className="rounded-md border border-[var(--crit)]/40 bg-[var(--crit)]/10 px-3 py-2 text-sm text-[var(--crit)]">
          {error}
        </div>
      )}

      {filtered.length === 0 ? (
        <div className="rounded-[var(--radius)] border border-dashed border-[var(--border-strong)] px-6 py-12 text-center text-sm text-[var(--text-1)]">
          {rows.length === 0 ? "No containers reported yet." : "No containers match this filter."}
        </div>
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
