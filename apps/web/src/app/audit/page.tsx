"use client";

import { useEffect, useState } from "react";
import { API_URL } from "@/lib/api";

type AuditRow = {
  id: string;
  ts: string;
  action: string;
  target_type: string;
  target_id: string;
  result: string;
  ip: string;
  user_email: string;
};

export default function AuditPage() {
  const [rows, setRows] = useState<AuditRow[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const boot = window.setTimeout(() => {
      fetch(`${API_URL}/api/v1/audit-logs`, { credentials: "include", cache: "no-store" })
        .then(async (r) => {
          if (!r.ok) throw new Error("Could not load audit log");
          return r.json();
        })
        .then((d) => setRows(d.data ?? []))
        .catch((e) => setError(e instanceof Error ? e.message : "Failed"));
    }, 0);
    return () => window.clearTimeout(boot);
  }, []);

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <h1 className="text-3xl font-semibold tracking-tight">Audit Log</h1>
      {error && (
        <div className="rounded-md border border-[var(--crit)]/40 bg-[var(--crit)]/10 px-3 py-2 text-sm text-[var(--crit)]">
          {error}
        </div>
      )}
      {rows.length === 0 && !error ? (
        <div className="rounded-[var(--radius)] border border-dashed border-[var(--border-strong)] px-5 py-10 text-center text-sm text-[var(--text-2)]">
          No audited actions yet.
        </div>
      ) : (
        <div className="overflow-hidden rounded-[var(--radius)] border border-[var(--border)]">
          <table className="w-full text-left text-sm">
            <thead className="bg-[var(--bg-2)] text-[11px] uppercase tracking-[0.12em] text-[var(--text-2)]">
              <tr>
                <th className="px-4 py-3">Time</th>
                <th className="px-4 py-3">User</th>
                <th className="px-4 py-3">Action</th>
                <th className="px-4 py-3">Target</th>
                <th className="px-4 py-3">Result</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.id} className="border-t border-[var(--border)] bg-[var(--bg-1)]">
                  <td className="px-4 py-3 font-[family-name:var(--font-mono-family)] text-xs text-[var(--text-2)]">
                    {new Date(r.ts).toLocaleString()}
                  </td>
                  <td className="px-4 py-3">{r.user_email || "—"}</td>
                  <td className="px-4 py-3">{r.action}</td>
                  <td className="px-4 py-3 text-[var(--text-2)]">
                    {[r.target_type, r.target_id].filter(Boolean).join(" ") || "—"}
                  </td>
                  <td className="px-4 py-3">{r.result}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
