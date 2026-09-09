"use client";

import { useMemo, useState } from "react";

export function InventoryTable({
  title,
  empty,
  headers,
  rows,
  searchable = true,
  searchPlaceholder = "Search…",
  loading = false,
  error = null,
}: {
  title: string;
  empty: string;
  headers: string[];
  rows: string[][];
  searchable?: boolean;
  searchPlaceholder?: string;
  loading?: boolean;
  error?: string | null;
}) {
  const [q, setQ] = useState("");

  const filtered = useMemo(() => {
    const needle = q.trim().toLowerCase();
    if (!needle) return rows;
    return rows.filter((r) => r.some((c) => (c || "").toLowerCase().includes(needle)));
  }, [rows, q]);

  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <h1 className="text-3xl font-semibold tracking-tight">{title}</h1>

      {searchable && (
        <input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder={searchPlaceholder}
          className="w-full rounded-md border border-[var(--border)] bg-[var(--bg-2)] px-3 py-2 text-sm"
          aria-label={searchPlaceholder}
        />
      )}

      {error && (
        <div
          role="alert"
          className="rounded-md border border-[var(--crit)]/40 bg-[var(--crit)]/10 px-3 py-2 text-sm text-[var(--crit)]"
        >
          {error}
        </div>
      )}

      {loading && rows.length === 0 && !error ? (
        <div
          className="h-40 animate-pulse rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-2)]"
          aria-busy="true"
          aria-label="Loading"
        />
      ) : filtered.length === 0 ? (
        <div className="rounded-[var(--radius)] border border-dashed border-[var(--border-strong)] px-5 py-10 text-center text-sm text-[var(--text-2)]">
          {rows.length === 0 ? empty : "No rows match this search."}
        </div>
      ) : (
        <div className="overflow-hidden rounded-[var(--radius)] border border-[var(--border)]">
          <table className="w-full text-left text-sm">
            <thead className="bg-[var(--bg-2)] text-[11px] uppercase tracking-[0.12em] text-[var(--text-2)]">
              <tr>
                {headers.map((h) => (
                  <th key={h} className="px-4 py-3 font-medium">
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {filtered.slice(0, 300).map((r, i) => (
                <tr key={i} className="border-t border-[var(--border)] bg-[var(--bg-1)]">
                  {r.map((c, j) => (
                    <td key={j} className="px-4 py-3 text-[var(--text-1)]">
                      {c || "—"}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
