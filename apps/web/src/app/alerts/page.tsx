"use client";

import { useEffect, useState } from "react";
import { API_URL, apiFetch } from "@/lib/api";
import { useRealtime } from "@/lib/realtime";

type Alert = {
  id: string;
  severity: string;
  status: string;
  server_id: string | null;
  message: string;
  first_seen_at: string;
  last_seen_at: string;
};

type Rule = {
  id: string;
  name: string;
  enabled: boolean;
  severity: string;
  metric: string;
  operator: string;
  threshold: number;
  duration_seconds: number;
};

const btnClass =
  "rounded-md border border-[var(--border)] bg-[var(--bg-2)] px-2.5 py-1 text-xs text-[var(--text-1)] hover:bg-[var(--bg-3)]";

export default function AlertsPage() {
  const [alerts, setAlerts] = useState<Alert[]>([]);
  const [rules, setRules] = useState<Rule[]>([]);
  const [error, setError] = useState<string | null>(null);

  async function load() {
    try {
      const [a, r] = await Promise.all([
        fetch(`${API_URL}/api/v1/alerts`, { credentials: "include", cache: "no-store" }).then((x) => x.json()),
        fetch(`${API_URL}/api/v1/alert-rules`, { credentials: "include", cache: "no-store" }).then((x) => x.json()),
      ]);
      setAlerts(a.data ?? []);
      setRules(r.data ?? []);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load alerts");
    }
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
    if (type === "alerts.updated" || type === "overview.delta") void load();
  });

  async function act(id: string, action: "acknowledge" | "resolve" | "silence") {
    await apiFetch(`/api/v1/alerts/${id}/${action}`, {
      method: "POST",
    });
    await load();
  }

  const active = alerts.filter((a) => a.status === "active" || a.status === "acknowledged");

  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <div>
        <div className="text-[11px] font-medium uppercase tracking-[0.18em] text-[var(--text-2)]">
          Observability
        </div>
        <h1 className="mt-1 text-3xl font-semibold tracking-tight">Alerts</h1>
        <p className="mt-1 text-sm text-[var(--text-1)]">
          Rules evaluate real metrics and inventory. No simulated firings.
        </p>
      </div>

      {error && (
        <div className="rounded-md border border-[var(--crit)]/40 bg-[var(--crit)]/10 px-3 py-2 text-sm text-[var(--crit)]">
          {error}
        </div>
      )}

      <section className="space-y-3">
        <h2 className="text-lg font-semibold">Active</h2>
        {active.length === 0 ? (
          <div className="rounded-[var(--radius)] border border-dashed border-[var(--border-strong)] px-5 py-8 text-center text-sm text-[var(--text-2)]">
            No active alerts.
          </div>
        ) : (
          <div className="space-y-2">
            {active.map((a) => (
              <div
                key={a.id}
                className="flex flex-col gap-3 rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-4 sm:flex-row sm:items-center sm:justify-between"
              >
                <div>
                  <div className="flex items-center gap-2 text-sm">
                    <Severity severity={a.severity} />
                    <span className="font-medium">{a.message}</span>
                  </div>
                  <div className="mt-1 text-xs text-[var(--text-2)]">
                    {a.status} · first {new Date(a.first_seen_at).toLocaleString()} · last{" "}
                    {new Date(a.last_seen_at).toLocaleString()}
                  </div>
                </div>
                <div className="flex gap-2">
                  <button type="button" className={btnClass} onClick={() => act(a.id, "acknowledge")}>
                    Acknowledge
                  </button>
                  <button type="button" className={btnClass} onClick={() => act(a.id, "silence")}>
                    Silence
                  </button>
                  <button type="button" className={btnClass} onClick={() => act(a.id, "resolve")}>
                    Resolve
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

      <section className="space-y-3">
        <h2 className="text-lg font-semibold">Rules</h2>
        <div className="overflow-hidden rounded-[var(--radius)] border border-[var(--border)]">
          <table className="w-full text-left text-sm">
            <thead className="bg-[var(--bg-2)] text-[11px] uppercase tracking-[0.12em] text-[var(--text-2)]">
              <tr>
                <th className="px-4 py-3">Name</th>
                <th className="px-4 py-3">Severity</th>
                <th className="px-4 py-3">Condition</th>
                <th className="px-4 py-3">Enabled</th>
              </tr>
            </thead>
            <tbody>
              {rules.map((r) => (
                <tr key={r.id} className="border-t border-[var(--border)] bg-[var(--bg-1)]">
                  <td className="px-4 py-3">{r.name}</td>
                  <td className="px-4 py-3">
                    <Severity severity={r.severity} />
                  </td>
                  <td className="px-4 py-3 font-[family-name:var(--font-mono-family)] text-xs text-[var(--text-2)]">
                    {r.metric} {r.operator} {r.threshold} for {r.duration_seconds}s
                  </td>
                  <td className="px-4 py-3">{r.enabled ? "yes" : "no"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}

function Severity({ severity }: { severity: string }) {
  const color =
    severity === "critical" ? "var(--crit)" : severity === "warning" ? "var(--warn)" : "var(--accent)";
  return (
    <span className="inline-flex items-center gap-1.5 text-xs uppercase tracking-wide" style={{ color }}>
      <span className="h-1.5 w-1.5 rounded-full" style={{ background: color }} aria-hidden />
      {severity}
    </span>
  );
}
