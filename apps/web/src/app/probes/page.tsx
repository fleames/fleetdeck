"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { apiFetch } from "@/lib/api";
import { StatusPill } from "@/components/status-pill";
import { useRealtime } from "@/lib/realtime";

type Probe = {
  id: string;
  name: string;
  enabled: boolean;
  kind: "http" | "tcp";
  target: string;
  method: string;
  expected_status: number;
  interval_seconds: number;
  timeout_ms: number;
  fail_threshold: number;
  severity: string;
  consecutive_fails: number;
  last_status: string;
  last_latency_ms: number | null;
  last_error: string;
  last_checked_at: string | null;
};

const emptyForm = {
  name: "",
  kind: "http" as "http" | "tcp",
  target: "",
  method: "GET",
  expected_status: 200,
  interval_seconds: 60,
  timeout_ms: 3000,
  fail_threshold: 3,
  severity: "warning",
  enabled: true,
};

export default function ProbesPage() {
  const [rows, setRows] = useState<Probe[]>([]);
  const [max, setMax] = useState(20);
  const [form, setForm] = useState(emptyForm);
  const [error, setError] = useState<string | null>(null);
  const [msg, setMsg] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const load = useCallback(() => {
    apiFetch("/api/v1/uptime-probes")
      .then(async (r) => {
        const body = await r.json();
        if (!r.ok) throw new Error(body?.error?.message ?? "Could not load probes");
        setRows(body.data ?? []);
        setMax(body.meta?.max ?? 20);
      })
      .catch((e) => setError(e instanceof Error ? e.message : "Failed to load probes"));
  }, []);

  useEffect(() => {
    const boot = window.setTimeout(load, 0);
    const poll = window.setInterval(load, 15000);
    return () => {
      window.clearTimeout(boot);
      window.clearInterval(poll);
    };
  }, [load]);

  useRealtime((type) => {
    if (type === "alerts.updated" || type === "overview.delta") load();
  });

  async function onCreate(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    setMsg(null);
    try {
      const res = await apiFetch("/api/v1/uptime-probes", {
        method: "POST",
        body: JSON.stringify(form),
      });
      const body = await res.json();
      if (!res.ok) throw new Error(body?.error?.message ?? "Create failed");
      setForm(emptyForm);
      setMsg(`Created probe “${body.name}”. Checks run from the control plane (not agents).`);
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Create failed");
    } finally {
      setBusy(false);
    }
  }

  async function toggleEnabled(p: Probe) {
    setError(null);
    const res = await apiFetch(`/api/v1/uptime-probes/${p.id}`, {
      method: "PATCH",
      body: JSON.stringify({
        name: p.name,
        kind: p.kind,
        target: p.target,
        method: p.method,
        expected_status: p.expected_status,
        interval_seconds: p.interval_seconds,
        timeout_ms: p.timeout_ms,
        fail_threshold: p.fail_threshold,
        severity: p.severity,
        enabled: !p.enabled,
      }),
    });
    if (!res.ok) {
      const body = await res.json().catch(() => null);
      setError(body?.error?.message ?? "Update failed");
      return;
    }
    load();
  }

  async function removeProbe(p: Probe) {
    if (!window.confirm(`Delete probe “${p.name}”?`)) return;
    setError(null);
    const res = await apiFetch(`/api/v1/uptime-probes/${p.id}`, { method: "DELETE" });
    if (!res.ok) {
      const body = await res.json().catch(() => null);
      setError(body?.error?.message ?? "Delete failed");
      return;
    }
    load();
  }

  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <div>
        <div className="text-[11px] font-medium uppercase tracking-[0.18em] text-[var(--text-2)]">
          Observability
        </div>
        <h1 className="mt-1 text-3xl font-semibold tracking-tight">Uptime probes</h1>
        <p className="mt-1 text-sm text-[var(--text-1)]">
          Lightweight HTTP/TCP checks from the FleetDeck API host (max {max}). Failures enter the
          same alert/webhook pipeline. These do <strong>not</strong> use agents or the tunnel.
        </p>
      </div>

      {error && (
        <div className="rounded-md border border-[var(--crit)]/40 bg-[var(--crit)]/10 px-3 py-2 text-sm text-[var(--crit)]">
          {error}
        </div>
      )}
      {msg && (
        <div className="rounded-md border border-[var(--ok)]/40 bg-[var(--ok)]/10 px-3 py-2 text-sm text-[var(--ok)]">
          {msg}
        </div>
      )}

      <form
        onSubmit={(e) => void onCreate(e)}
        className="space-y-3 rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-4"
      >
        <div className="text-sm font-medium">Add probe</div>
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <label className="text-xs text-[var(--text-2)]">
            Name
            <input
              required
              className="mt-1 w-full rounded-md border border-[var(--border)] bg-[var(--bg-2)] px-3 py-2 text-sm"
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
            />
          </label>
          <label className="text-xs text-[var(--text-2)]">
            Kind
            <select
              className="mt-1 w-full rounded-md border border-[var(--border)] bg-[var(--bg-2)] px-3 py-2 text-sm"
              value={form.kind}
              onChange={(e) =>
                setForm({
                  ...form,
                  kind: e.target.value as "http" | "tcp",
                  target: "",
                })
              }
            >
              <option value="http">HTTP</option>
              <option value="tcp">TCP</option>
            </select>
          </label>
          <label className="text-xs text-[var(--text-2)] sm:col-span-2">
            Target {form.kind === "http" ? "(https://…)" : "(host:port)"}
            <input
              required
              placeholder={form.kind === "http" ? "https://example.com/healthz" : "192.168.1.10:5432"}
              className="mt-1 w-full rounded-md border border-[var(--border)] bg-[var(--bg-2)] px-3 py-2 text-sm font-[family-name:var(--font-mono-family)]"
              value={form.target}
              onChange={(e) => setForm({ ...form, target: e.target.value })}
            />
          </label>
          {form.kind === "http" && (
            <>
              <label className="text-xs text-[var(--text-2)]">
                Method
                <select
                  className="mt-1 w-full rounded-md border border-[var(--border)] bg-[var(--bg-2)] px-3 py-2 text-sm"
                  value={form.method}
                  onChange={(e) => setForm({ ...form, method: e.target.value })}
                >
                  <option value="GET">GET</option>
                  <option value="HEAD">HEAD</option>
                </select>
              </label>
              <label className="text-xs text-[var(--text-2)]">
                Expected status
                <input
                  type="number"
                  className="mt-1 w-full rounded-md border border-[var(--border)] bg-[var(--bg-2)] px-3 py-2 text-sm"
                  value={form.expected_status}
                  onChange={(e) => setForm({ ...form, expected_status: Number(e.target.value) })}
                />
              </label>
            </>
          )}
          <label className="text-xs text-[var(--text-2)]">
            Interval (sec)
            <input
              type="number"
              min={30}
              max={600}
              className="mt-1 w-full rounded-md border border-[var(--border)] bg-[var(--bg-2)] px-3 py-2 text-sm"
              value={form.interval_seconds}
              onChange={(e) => setForm({ ...form, interval_seconds: Number(e.target.value) })}
            />
          </label>
          <label className="text-xs text-[var(--text-2)]">
            Timeout (ms)
            <input
              type="number"
              min={500}
              max={10000}
              className="mt-1 w-full rounded-md border border-[var(--border)] bg-[var(--bg-2)] px-3 py-2 text-sm"
              value={form.timeout_ms}
              onChange={(e) => setForm({ ...form, timeout_ms: Number(e.target.value) })}
            />
          </label>
          <label className="text-xs text-[var(--text-2)]">
            Fail threshold
            <input
              type="number"
              min={1}
              max={10}
              className="mt-1 w-full rounded-md border border-[var(--border)] bg-[var(--bg-2)] px-3 py-2 text-sm"
              value={form.fail_threshold}
              onChange={(e) => setForm({ ...form, fail_threshold: Number(e.target.value) })}
            />
          </label>
          <label className="text-xs text-[var(--text-2)]">
            Severity
            <select
              className="mt-1 w-full rounded-md border border-[var(--border)] bg-[var(--bg-2)] px-3 py-2 text-sm"
              value={form.severity}
              onChange={(e) => setForm({ ...form, severity: e.target.value })}
            >
              <option value="info">info</option>
              <option value="warning">warning</option>
              <option value="critical">critical</option>
            </select>
          </label>
        </div>
        <button
          type="submit"
          disabled={busy || rows.length >= max}
          className="rounded-md bg-[var(--accent)] px-4 py-2 text-sm text-white disabled:opacity-50"
        >
          {busy ? "Adding…" : "Add probe"}
        </button>
      </form>

      {rows.length === 0 ? (
        <div className="rounded-[var(--radius)] border border-dashed border-[var(--border-strong)] px-5 py-10 text-center text-sm text-[var(--text-2)]">
          No probes yet. Add a few critical HTTP/TCP checks (panel health, DB port, public site).
        </div>
      ) : (
        <div className="overflow-hidden rounded-[var(--radius)] border border-[var(--border)]">
          <table className="w-full text-left text-sm">
            <thead className="bg-[var(--bg-2)] text-[11px] uppercase tracking-[0.12em] text-[var(--text-2)]">
              <tr>
                <th className="px-4 py-3 font-medium">Name</th>
                <th className="px-4 py-3 font-medium">Target</th>
                <th className="px-4 py-3 font-medium">Status</th>
                <th className="px-4 py-3 font-medium">Latency</th>
                <th className="px-4 py-3 font-medium">Every</th>
                <th className="px-4 py-3 font-medium">Actions</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((p) => (
                <tr key={p.id} className="border-t border-[var(--border)] bg-[var(--bg-1)]">
                  <td className="px-4 py-3">
                    <div className="font-medium">{p.name}</div>
                    <div className="text-[11px] uppercase text-[var(--text-2)]">
                      {p.kind}
                      {!p.enabled ? " · disabled" : ""}
                    </div>
                  </td>
                  <td className="px-4 py-3 font-[family-name:var(--font-mono-family)] text-xs text-[var(--text-1)]">
                    {p.target}
                    {p.last_error ? (
                      <div className="mt-1 text-[var(--crit)]">{p.last_error}</div>
                    ) : null}
                  </td>
                  <td className="px-4 py-3">
                    <StatusPill
                      state={
                        p.last_status === "up"
                          ? "healthy"
                          : p.last_status === "down"
                            ? "critical"
                            : "unknown"
                      }
                    />
                    {p.consecutive_fails > 0 && (
                      <div className="mt-1 text-[11px] text-[var(--warn)]">
                        {p.consecutive_fails}/{p.fail_threshold} fails
                      </div>
                    )}
                  </td>
                  <td className="px-4 py-3 text-[var(--text-1)]">
                    {p.last_latency_ms == null ? "—" : `${p.last_latency_ms} ms`}
                  </td>
                  <td className="px-4 py-3 text-[var(--text-1)]">{p.interval_seconds}s</td>
                  <td className="px-4 py-3">
                    <div className="flex flex-wrap gap-2">
                      <button
                        type="button"
                        className="rounded border border-[var(--border)] px-2 py-0.5 text-[11px]"
                        onClick={() => void toggleEnabled(p)}
                      >
                        {p.enabled ? "Disable" : "Enable"}
                      </button>
                      <button
                        type="button"
                        className="rounded border border-[var(--crit)]/40 px-2 py-0.5 text-[11px] text-[var(--crit)]"
                        onClick={() => void removeProbe(p)}
                      >
                        Delete
                      </button>
                    </div>
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
