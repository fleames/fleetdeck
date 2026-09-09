"use client";

import { useCallback, useEffect, useState } from "react";
import { API_URL } from "@/lib/api";
import { useRealtime } from "@/lib/realtime";
import { EmptyBlock, ErrorBanner, LoadingBlock } from "@/components/page-state";

type EventRow = {
  id: string;
  ts: string;
  kind: string;
  severity: string;
  server_id: string | null;
  message: string;
};

export default function EventsPage() {
  const [rows, setRows] = useState<EventRow[]>([]);
  const [q, setQ] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async (opts?: { soft?: boolean }) => {
    try {
      const res = await fetch(`${API_URL}/api/v1/events`, { credentials: "include", cache: "no-store" });
      if (!res.ok) throw new Error(`Events request failed (${res.status})`);
      const data = await res.json();
      setRows(data.data ?? []);
      setError(null);
    } catch (e) {
      if (!opts?.soft) setError(e instanceof Error ? e.message : "Failed to load events");
    } finally {
      if (!opts?.soft) setLoading(false);
    }
  }, []);

  useEffect(() => {
    const boot = window.setTimeout(() => void load(), 0);
    const poll = window.setInterval(() => void load({ soft: true }), 10000);
    return () => {
      window.clearTimeout(boot);
      window.clearInterval(poll);
    };
  }, [load]);

  useRealtime((type) => {
    if (type.startsWith("alert") || type === "servers.updated" || type === "docker.updated") {
      void load({ soft: true });
    }
  });

  const filtered = rows.filter((e) => {
    const needle = q.trim().toLowerCase();
    if (!needle) return true;
    return e.message.toLowerCase().includes(needle) || e.kind.toLowerCase().includes(needle);
  });

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <div>
        <div className="text-[11px] font-medium uppercase tracking-[0.18em] text-[var(--text-2)]">
          Timeline
        </div>
        <h1 className="mt-1 text-3xl font-semibold tracking-tight">Events</h1>
      </div>
      <input
        value={q}
        onChange={(e) => setQ(e.target.value)}
        placeholder="Search events…"
        className="w-full rounded-md border border-[var(--border)] bg-[var(--bg-2)] px-3 py-2 text-sm"
      />
      {error && <ErrorBanner message={error} />}
      {loading && rows.length === 0 && !error ? (
        <LoadingBlock label="Loading events" />
      ) : filtered.length === 0 ? (
        <EmptyBlock>
          {rows.length === 0 ? "No infrastructure events yet." : "No events match this search."}
        </EmptyBlock>
      ) : (
        <ol className="space-y-2">
          {filtered.map((e) => (
            <li
              key={e.id}
              className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] px-4 py-3 text-sm"
            >
              <div className="flex flex-wrap items-center gap-3">
                <span className="font-[family-name:var(--font-mono-family)] text-xs text-[var(--text-2)]">
                  {new Date(e.ts).toLocaleString()}
                </span>
                <span className="text-xs uppercase tracking-wide text-[var(--text-2)]">{e.severity}</span>
                <span className="text-xs text-[var(--accent)]">{e.kind}</span>
              </div>
              <div className="mt-1 text-[var(--text-0)]">{e.message}</div>
            </li>
          ))}
        </ol>
      )}
    </div>
  );
}
