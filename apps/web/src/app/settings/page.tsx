"use client";

import { FormEvent, useEffect, useState } from "react";
import { API_URL, apiFetch } from "@/lib/api";

type Settings = {
  general?: { app_name?: string; theme?: string; timezone?: string };
  metrics?: {
    interval_seconds?: number;
    raw_retention_days?: number;
    agg_5m_retention_days?: number;
    agg_1h_retention_days?: number;
  };
  alerts?: {
    defaults_enabled?: boolean;
    webhook_url?: string;
    webhook_format?: "auto" | "json" | "discord" | "slack";
  };
};

const fieldClass =
  "w-full rounded-lg border border-[var(--border)] bg-[var(--bg-2)] px-3 py-2 text-sm text-[var(--text-0)]";

const btnSecondary =
  "rounded-md border border-[var(--border)] px-3 py-1.5 text-xs text-[var(--text-1)] hover:bg-[var(--bg-2)]";

export default function SettingsPage() {
  const [settings, setSettings] = useState<Settings>({});
  const [msg, setMsg] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [restoreText, setRestoreText] = useState("");
  const [confirmRestore, setConfirmRestore] = useState(false);
  const [signingSecret, setSigningSecret] = useState("");
  const [alertSigningConfigured, setAlertSigningConfigured] = useState(false);

  async function load() {
    const res = await fetch(`${API_URL}/api/v1/settings`, { credentials: "include", cache: "no-store" });
    if (!res.ok) throw new Error("Could not load settings");
    setSettings(await res.json());
    const sec = await fetch(`${API_URL}/api/v1/secrets/status`, { credentials: "include", cache: "no-store" });
    if (sec.ok) {
      const body = (await sec.json()) as { alert_signing?: boolean };
      setAlertSigningConfigured(Boolean(body.alert_signing));
    }
  }

  useEffect(() => {
    const boot = window.setTimeout(() => {
      load().catch((e) => setError(e instanceof Error ? e.message : "Failed"));
    }, 0);
    return () => window.clearTimeout(boot);
  }, []);

  function applyTheme(theme?: string) {
    document.documentElement.setAttribute("data-theme", theme === "light" ? "light" : "dark");
  }

  async function save(e: FormEvent) {
    e.preventDefault();
    setMsg(null);
    setError(null);
    const res = await apiFetch(`/api/v1/settings`, {
      method: "PATCH",
      body: JSON.stringify(settings),
    });
    if (!res.ok) {
      setError("Save failed");
      return;
    }
    const next = await res.json();
    setSettings(next);
    applyTheme(next.general?.theme);
    setMsg("Settings saved.");
  }

  async function saveSigningSecret() {
    setError(null);
    setMsg(null);
    const value = signingSecret.trim();
    if (!value) {
      setError("Enter a signing secret before saving.");
      return;
    }
    const res = await apiFetch(`/api/v1/secrets/webhook/alert_signing`, {
      method: "PUT",
      body: JSON.stringify({ value }),
    });
    if (!res.ok) {
      const body = await res.json().catch(() => null);
      setError(body?.error?.message || "Could not store signing secret");
      return;
    }
    setSigningSecret("");
    setAlertSigningConfigured(true);
    setMsg("Webhook signing secret stored (envelope-encrypted). Applied to generic JSON webhooks as X-FleetDeck-Signature.");
  }

  async function clearSigningSecret() {
    setError(null);
    setMsg(null);
    const res = await apiFetch(`/api/v1/secrets/webhook/alert_signing`, { method: "DELETE" });
    if (!res.ok) {
      setError("Could not clear signing secret");
      return;
    }
    setAlertSigningConfigured(false);
    setMsg("Webhook signing secret removed.");
  }

  async function downloadExport(kind: string, format: "json" | "csv") {
    setError(null);
    const res = await fetch(`${API_URL}/api/v1/export/${kind}?format=${format}`, {
      credentials: "include",
    });
    if (!res.ok) {
      setError(`Export ${kind} failed`);
      return;
    }
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `fleetdeck-${kind}.${format}`;
    a.click();
    URL.revokeObjectURL(url);
  }

  async function downloadBackup() {
    setError(null);
    const res = await apiFetch(`/api/v1/backup`, {
      method: "POST",
    });
    if (!res.ok) {
      setError("Backup failed");
      return;
    }
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `fleetdeck-backup.json`;
    a.click();
    URL.revokeObjectURL(url);
    setMsg("Backup downloaded (settings, server registry metadata, alert rules, layouts).");
  }

  async function runRestore() {
    setError(null);
    setMsg(null);
    let payload: Record<string, unknown>;
    try {
      payload = JSON.parse(restoreText) as Record<string, unknown>;
    } catch {
      setError("Restore JSON is invalid.");
      return;
    }
    if (!confirmRestore) {
      setError("Check confirm restore before applying.");
      return;
    }
    const res = await apiFetch(`/api/v1/restore`, {
      method: "POST",
      body: JSON.stringify({ ...payload, confirm: true }),
    });
    if (!res.ok) {
      const body = await res.json().catch(() => null);
      setError(body?.error?.message || "Restore failed");
      return;
    }
    const result = await res.json();
    const nextRes = await fetch(`${API_URL}/api/v1/settings`, {
      credentials: "include",
      cache: "no-store",
    });
    if (nextRes.ok) {
      const next = (await nextRes.json()) as Settings;
      setSettings(next);
      applyTheme(next.general?.theme);
    }
    setMsg(
      `Restore applied: ${result.settings_restored ?? 0} settings, ${result.alert_rules_added ?? 0} rules. Re-enroll agents if needed.`,
    );
  }

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div>
        <h1 className="text-3xl font-semibold tracking-tight">Settings</h1>
        <p className="mt-1 text-sm text-[var(--text-1)]">
          Local configuration. Secret values are write-only (never displayed after save).
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
        onSubmit={save}
        className="space-y-4 rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-5"
      >
        <label className="block text-xs text-[var(--text-2)]">
          <span className="mb-1.5 block">Application name</span>
          <input
            className={fieldClass}
            value={settings.general?.app_name ?? "FleetDeck"}
            onChange={(e) =>
              setSettings((s) => ({ ...s, general: { ...s.general, app_name: e.target.value } }))
            }
          />
        </label>
        <label className="block text-xs text-[var(--text-2)]">
          <span className="mb-1.5 block">Theme</span>
          <select
            className={fieldClass}
            value={settings.general?.theme ?? "dark"}
            onChange={(e) => {
              const theme = e.target.value;
              setSettings((s) => ({ ...s, general: { ...s.general, theme } }));
              applyTheme(theme);
            }}
          >
            <option value="dark">Dark</option>
            <option value="light">Light</option>
          </select>
        </label>
        <label className="block text-xs text-[var(--text-2)]">
          <span className="mb-1.5 block">Timezone</span>
          <input
            className={fieldClass}
            value={settings.general?.timezone ?? "UTC"}
            onChange={(e) =>
              setSettings((s) => ({ ...s, general: { ...s.general, timezone: e.target.value } }))
            }
          />
        </label>
        <label className="block text-xs text-[var(--text-2)]">
          <span className="mb-1.5 block">Raw metrics retention (days)</span>
          <input
            className={fieldClass}
            type="number"
            min={1}
            value={settings.metrics?.raw_retention_days ?? 7}
            onChange={(e) =>
              setSettings((s) => ({
                ...s,
                metrics: { ...s.metrics, raw_retention_days: Number(e.target.value) },
              }))
            }
          />
        </label>
        <label className="block text-xs text-[var(--text-2)]">
          <span className="mb-1.5 block">5-minute aggregate retention (days)</span>
          <input
            className={fieldClass}
            type="number"
            min={1}
            value={settings.metrics?.agg_5m_retention_days ?? 30}
            onChange={(e) =>
              setSettings((s) => ({
                ...s,
                metrics: { ...s.metrics, agg_5m_retention_days: Number(e.target.value) },
              }))
            }
          />
        </label>
        <label className="block text-xs text-[var(--text-2)]">
          <span className="mb-1.5 block">1-hour aggregate retention (days)</span>
          <input
            className={fieldClass}
            type="number"
            min={1}
            value={settings.metrics?.agg_1h_retention_days ?? 365}
            onChange={(e) =>
              setSettings((s) => ({
                ...s,
                metrics: { ...s.metrics, agg_1h_retention_days: Number(e.target.value) },
              }))
            }
          />
        </label>
        <p className="text-xs text-[var(--text-2)]">
          Retention is applied hourly by the API worker (DELETE on current partitions). Env vars
          <code className="mx-1">METRICS_*_RETENTION_DAYS</code>
          are the defaults when settings are unset. Restoring Settings JSON alone does not restore deleted metrics — use Postgres volume /
          <code className="mx-1">pg_dump</code>.
        </p>
        <label className="block text-xs text-[var(--text-2)]">
          <span className="mb-1.5 block">Alert webhook URL</span>
          <input
            className={fieldClass}
            type="url"
            placeholder="https://hooks.slack.com/... or Discord webhook / custom JSON endpoint"
            value={settings.alerts?.webhook_url ?? ""}
            onChange={(e) =>
              setSettings((s) => ({
                ...s,
                alerts: { ...s.alerts, webhook_url: e.target.value },
              }))
            }
          />
        </label>
        <label className="block text-xs text-[var(--text-2)]">
          <span className="mb-1.5 block">Webhook format</span>
          <select
            className={fieldClass}
            value={settings.alerts?.webhook_format ?? "auto"}
            onChange={(e) =>
              setSettings((s) => ({
                ...s,
                alerts: {
                  ...s.alerts,
                  webhook_format: e.target.value as "auto" | "json" | "discord" | "slack",
                },
              }))
            }
          >
            <option value="auto">Auto (detect Discord/Slack from URL)</option>
            <option value="json">Generic JSON</option>
            <option value="discord">Discord embeds</option>
            <option value="slack">Slack incoming webhook</option>
          </select>
        </label>
        <p className="text-xs text-[var(--text-2)]">
          Overrides <code className="mx-1">ALERT_WEBHOOK_URL</code> when set. Fire/resolve POSTs use Discord or Slack
          shaping when auto-detected; otherwise FleetDeck JSON. Email remains deferred.
        </p>
        <button type="submit" className="rounded-md bg-[var(--accent)] px-4 py-2 text-sm text-white">
          Save
        </button>
      </form>

      <section className="space-y-3 rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-5">
        <h2 className="text-sm font-medium text-[var(--text-0)]">Webhook signing secret</h2>
        <p className="text-xs text-[var(--text-2)]">
          Optional HMAC key for generic JSON webhooks (
          <code className="mx-1">X-FleetDeck-Signature: sha256=…</code>
          ). Stored in the AES-GCM envelope (
          <code className="mx-1">secrets</code> table), never returned. Status:{" "}
          {alertSigningConfigured ? "configured" : "not set"}.
        </p>
        <label className="block text-xs text-[var(--text-2)]">
          <span className="mb-1.5 block">New signing secret</span>
          <input
            className={fieldClass}
            type="password"
            autoComplete="new-password"
            value={signingSecret}
            onChange={(e) => setSigningSecret(e.target.value)}
            placeholder={alertSigningConfigured ? "•••••••• (enter to rotate)" : "Optional shared secret"}
          />
        </label>
        <div className="flex flex-wrap gap-2">
          <button type="button" className={btnSecondary} onClick={() => void saveSigningSecret()}>
            Store / rotate
          </button>
          {alertSigningConfigured && (
            <button
              type="button"
              className="rounded-md border border-[var(--crit)]/40 px-3 py-1.5 text-xs text-[var(--crit)] hover:bg-[var(--crit)]/10"
              onClick={() => void clearSigningSecret()}
            >
              Clear
            </button>
          )}
        </div>
      </section>

      <section className="space-y-3 rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-5">
        <h2 className="text-sm font-medium text-[var(--text-0)]">Export</h2>
        <p className="text-xs text-[var(--text-2)]">Download fleet lists as CSV or JSON (no secrets).</p>
        <div className="flex flex-wrap gap-2">
          {(["servers", "containers", "alerts", "events"] as const).map((kind) => (
            <div key={kind} className="flex gap-1">
              <button type="button" className={btnSecondary} onClick={() => void downloadExport(kind, "csv")}>
                {kind} CSV
              </button>
              <button type="button" className={btnSecondary} onClick={() => void downloadExport(kind, "json")}>
                JSON
              </button>
            </div>
          ))}
        </div>
      </section>

      <section className="space-y-3 rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-5">
        <h2 className="text-sm font-medium text-[var(--text-0)]">Backup & restore</h2>
        <p className="text-xs text-[var(--text-2)]">
          Settings, server registry metadata, alert rules, and dashboard layouts. Historical metrics are
          excluded. Restore requires explicit confirmation and does not silently overwrite live fleet
          enrollments.
        </p>
        <button type="button" className={btnSecondary} onClick={() => void downloadBackup()}>
          Download backup JSON
        </button>
        <label className="block text-xs text-[var(--text-2)]">
          <span className="mb-1.5 block">Paste backup JSON to restore</span>
          <textarea
            className={`${fieldClass} min-h-28 font-[family-name:var(--font-mono-family)]`}
            value={restoreText}
            onChange={(e) => setRestoreText(e.target.value)}
            placeholder='{"version":1,"settings":{...},"alert_rules":[...]}'
          />
        </label>
        <label className="flex items-center gap-2 text-xs text-[var(--text-1)]">
          <input
            type="checkbox"
            checked={confirmRestore}
            onChange={(e) => setConfirmRestore(e.target.checked)}
          />
          I confirm restore (settings + add alert rules)
        </label>
        <button
          type="button"
          className="rounded-md border border-[var(--warn)]/50 px-3 py-1.5 text-xs text-[var(--warn)] hover:bg-[var(--warn)]/10"
          onClick={() => void runRestore()}
        >
          Restore
        </button>
      </section>
    </div>
  );
}
