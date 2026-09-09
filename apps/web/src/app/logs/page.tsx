"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { API_URL, apiFetch } from "@/lib/api";

type ContainerRow = {
  id: string;
  name: string;
  server_name: string;
  state: string;
  image_ref: string;
};

function levelClass(line: string): string {
  const u = line.toUpperCase();
  if (u.includes("ERROR") || u.includes("FATAL") || u.includes("CRITICAL")) return "text-[var(--crit)]";
  if (u.includes("WARN")) return "text-[var(--warn)]";
  if (u.includes("DEBUG")) return "text-[var(--text-2)]";
  return "text-[var(--text-1)]";
}

export default function LogsPage() {
  const [containers, setContainers] = useState<ContainerRow[]>([]);
  const [selected, setSelected] = useState("");
  const [lines, setLines] = useState<string[]>([]);
  const [filter, setFilter] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    const boot = window.setTimeout(() => {
      fetch(`${API_URL}/api/v1/containers`, { credentials: "include", cache: "no-store" })
        .then((r) => (r.ok ? r.json() : null))
        .then((d) => {
          const rows = (d?.data ?? []) as ContainerRow[];
          setContainers(rows);
          if (rows[0]) setSelected(rows[0].id);
        })
        .catch((e) => setError(e instanceof Error ? e.message : "Failed to load containers"));
    }, 0);
    return () => window.clearTimeout(boot);
  }, []);

  async function fetchLogs() {
    if (!selected) return;
    setBusy(true);
    setError(null);
    try {
      const res = await apiFetch(`/api/v1/containers/${selected}/logs?tail=300`);
      const data = await res.json();
      if (!res.ok) throw new Error(data?.error?.message ?? "Could not fetch logs");
      setLines(data.lines ?? []);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Log fetch failed");
      setLines([]);
    } finally {
      setBusy(false);
    }
  }

  const filtered = lines.filter((l) => !filter || l.toLowerCase().includes(filter.toLowerCase()));
  const current = containers.find((c) => c.id === selected);

  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <div>
        <div className="text-[11px] font-medium uppercase tracking-[0.18em] text-[var(--text-2)]">
          Observability
        </div>
        <h1 className="mt-1 text-3xl font-semibold tracking-tight">Logs</h1>
        <p className="mt-1 text-sm text-[var(--text-1)]">
          Container logs via the host agent. Rendered as plain text only — never as HTML.
        </p>
      </div>

      <section className="flex flex-wrap items-end gap-3 rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-4">
        <label className="min-w-[240px] flex-1 text-xs text-[var(--text-2)]">
          <span className="mb-1.5 block">Container</span>
          <select
            className="w-full rounded-md border border-[var(--border)] bg-[var(--bg-2)] px-3 py-2 text-sm text-[var(--text-0)]"
            value={selected}
            onChange={(e) => setSelected(e.target.value)}
          >
            {containers.length === 0 && <option value="">No containers inventoried</option>}
            {containers.map((c) => (
              <option key={c.id} value={c.id}>
                {c.server_name} · {c.name || c.id.slice(0, 8)} ({c.state})
              </option>
            ))}
          </select>
        </label>
        <input
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          placeholder="Filter lines…"
          className="min-w-[160px] flex-1 rounded-md border border-[var(--border)] bg-[var(--bg-2)] px-3 py-2 text-sm"
        />
        <button
          type="button"
          disabled={!selected || busy}
          onClick={() => void fetchLogs()}
          className="rounded-md bg-[var(--accent)] px-4 py-2 text-sm text-white disabled:opacity-50"
        >
          {busy ? "Fetching…" : "Fetch logs"}
        </button>
        {current && (
          <Link href={`/containers/${current.id}`} className="text-xs text-[var(--accent)] hover:underline">
            Open detail
          </Link>
        )}
      </section>

      {error && (
        <div className="rounded-md border border-[var(--crit)]/40 bg-[var(--crit)]/10 px-3 py-2 text-sm text-[var(--crit)]">
          {error}
        </div>
      )}

      <pre className="max-h-[560px] overflow-auto rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-0)] p-4 font-[family-name:var(--font-mono-family)] text-xs">
        {filtered.length === 0
          ? "No log lines loaded. Select a container and fetch from the agent."
          : filtered.map((line, i) => (
              <div key={i} className={levelClass(line)}>
                {line}
              </div>
            ))}
      </pre>
    </div>
  );
}
