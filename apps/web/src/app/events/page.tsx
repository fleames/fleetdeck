"use client";

import { useEffect, useState } from "react";
import { API_URL } from "@/lib/api";
import { useRealtime } from "@/lib/realtime";

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

  async function load() {
    const res = await fetch(`${API_URL}/api/v1/events`, { credentials: "include", cache: "no-store" });
    const data = await res.json();
    setRows(data.data ?? []);
  }

  useEffect(() => {
    const boot = window.setTimeout(() => void load(), 0);
    const poll = window.setInterval(() => void load(), 10000);
    return () => {
      window.clearTimeout(boot);
      window.clearInterval(poll);
    };
  }, []);

  useRealtime((type) => {
    if (type.startsWith("alert") || type === "servers.updated" || type === "docker.updated") void load();
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
      {filtered.length === 0 ? (
        <div className="rounded-[var(--radius)] border border-dashed border-[var(--border-strong)] px-5 py-10 text-center text-sm text-[var(--text-2)]">
          No infrastructure events yet.
        </div>
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
