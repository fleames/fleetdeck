"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { API_URL } from "@/lib/api";
import { ContainerLogViewer } from "@/components/container-log-viewer";

type ContainerRow = {
  id: string;
  name: string;
  server_name: string;
  state: string;
  image_ref: string;
};

export default function LogsPage() {
  const [containers, setContainers] = useState<ContainerRow[]>([]);
  const [selected, setSelected] = useState("");
  const [error, setError] = useState<string | null>(null);

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

  const current = containers.find((c) => c.id === selected);

  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <div>
        <div className="text-[11px] font-medium uppercase tracking-[0.18em] text-[var(--text-2)]">
          Observability
        </div>
        <h1 className="mt-1 text-3xl font-semibold tracking-tight">Logs</h1>
        <p className="mt-1 text-sm text-[var(--text-1)]">
          Container logs via the host agent. Follow, filter, and severity chips — plain text only.
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

      {selected ? (
        <ContainerLogViewer key={selected} containerId={selected} />
      ) : (
        <div className="rounded-[var(--radius)] border border-dashed border-[var(--border-strong)] px-5 py-10 text-center text-sm text-[var(--text-2)]">
          Select a container to load logs.
        </div>
      )}
    </div>
  );
}
