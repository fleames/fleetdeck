"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { API_URL, apiFetch } from "@/lib/api";
import { pct, formatBytes } from "@/lib/format";
import { StatusPill } from "@/components/status-pill";

type CompareRow = {
  id: string;
  name: string;
  status: string;
  health_state: string;
  running_containers: number;
  capacity_notes: string[];
  metrics?: {
    cpu_pct?: number;
    mem_used_bytes?: number;
    mem_total_bytes?: number;
    disk_used_bytes?: number;
    disk_total_bytes?: number;
    last_updated?: string;
  } | null;
};

type ServerOpt = { id: string; name: string };

export default function ComparePage() {
  const [options, setOptions] = useState<ServerOpt[]>([]);
  const [selected, setSelected] = useState<string[]>([]);
  const [rows, setRows] = useState<CompareRow[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    const boot = window.setTimeout(() => {
      fetch(`${API_URL}/api/v1/servers`, { credentials: "include", cache: "no-store" })
        .then((r) => (r.ok ? r.json() : null))
        .then((d) => {
          const list = (d?.data ?? []) as ServerOpt[];
          setOptions(list.map((s) => ({ id: s.id, name: s.name })));
        })
        .catch(() => undefined);
    }, 0);
    return () => window.clearTimeout(boot);
  }, []);

  function toggle(id: string) {
    setSelected((prev) => {
      if (prev.includes(id)) return prev.filter((x) => x !== id);
      if (prev.length >= 8) return prev;
      return [...prev, id];
    });
  }

  async function runCompare() {
    setBusy(true);
    setError(null);
    try {
      const res = await apiFetch(`/api/v1/servers/compare`, {
        method: "POST",
        body: JSON.stringify({ server_ids: selected }),
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data?.error?.message ?? "Compare failed");
      setRows(data.data ?? []);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Compare failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <div>
        <h1 className="text-3xl font-semibold tracking-tight">Compare servers</h1>
        <p className="mt-1 text-sm text-[var(--text-1)]">
          Side-by-side live metrics and capacity notes from real samples — never synthetic.
        </p>
      </div>

      <section className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-4">
        <div className="text-xs text-[var(--text-2)]">Select 2–8 servers</div>
        <div className="mt-3 flex flex-wrap gap-2">
          {options.map((o) => {
            const on = selected.includes(o.id);
            return (
              <button
                key={o.id}
                type="button"
                onClick={() => toggle(o.id)}
                className={`rounded-md border px-3 py-1.5 text-xs ${
                  on
                    ? "border-[var(--accent)] bg-[var(--accent)]/15 text-[var(--text-0)]"
                    : "border-[var(--border)] text-[var(--text-1)] hover:bg-[var(--bg-2)]"
                }`}
              >
                {o.name}
              </button>
            );
          })}
        </div>
        <button
          type="button"
          disabled={busy || selected.length < 2}
          onClick={() => void runCompare()}
          className="mt-4 rounded-md bg-[var(--accent)] px-4 py-2 text-sm text-white disabled:opacity-50"
        >
          {busy ? "Comparing…" : "Compare"}
        </button>
      </section>

      {error && (
        <div className="rounded-md border border-[var(--crit)]/40 bg-[var(--crit)]/10 px-3 py-2 text-sm text-[var(--crit)]">
          {error}
        </div>
      )}

      {rows.length > 0 && (
        <div className="overflow-x-auto rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)]">
          <table className="w-full min-w-[720px] text-left text-sm">
            <thead className="border-b border-[var(--border)] text-xs text-[var(--text-2)]">
              <tr>
                <th className="px-4 py-2 font-medium">Server</th>
                <th className="px-4 py-2 font-medium">Status</th>
                <th className="px-4 py-2 font-medium">CPU</th>
                <th className="px-4 py-2 font-medium">Memory</th>
                <th className="px-4 py-2 font-medium">Disk</th>
                <th className="px-4 py-2 font-medium">Containers</th>
                <th className="px-4 py-2 font-medium">Capacity</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.id} className="border-b border-[var(--border)] last:border-0">
                  <td className="px-4 py-3">
                    <Link href={`/servers/${r.id}`} className="text-[var(--accent)] hover:underline">
                      {r.name}
                    </Link>
                  </td>
                  <td className="px-4 py-3">
                    <StatusPill state={r.status} />
                  </td>
                  <td className="px-4 py-3">{fmtPct(r.metrics?.cpu_pct)}</td>
                  <td className="px-4 py-3">
                    {fmtPct(pct(r.metrics?.mem_used_bytes, r.metrics?.mem_total_bytes))}
                    <div className="text-[10px] text-[var(--text-2)]">
                      {formatBytes(r.metrics?.mem_used_bytes)} / {formatBytes(r.metrics?.mem_total_bytes)}
                    </div>
                  </td>
                  <td className="px-4 py-3">
                    {fmtPct(pct(r.metrics?.disk_used_bytes, r.metrics?.disk_total_bytes))}
                  </td>
                  <td className="px-4 py-3">{r.running_containers}</td>
                  <td className="px-4 py-3 text-xs text-[var(--text-1)]">
                    {(r.capacity_notes ?? []).join(" · ")}
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

function fmtPct(n: number | null | undefined) {
  if (n == null || Number.isNaN(n)) return "—";
  return `${n.toFixed(1)}%`;
}
