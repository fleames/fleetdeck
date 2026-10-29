"use client";

import Link from "next/link";
import { FormEvent, useCallback, useEffect, useState } from "react";
import { apiFetch } from "@/lib/api";
import { agentVersionOutdated } from "@/lib/agent-version";
import { StatusPill } from "@/components/status-pill";
import { useRealtime } from "@/lib/realtime";

type RmmHost = {
  id: string;
  name: string;
  hostname: string;
  os_name: string;
  os_version: string;
  arch: string;
  agent_version: string;
  status: string;
  rdp_enabled: boolean;
  rdp_port: number;
  last_heartbeat_at: string | null;
};

export default function RmmPage() {
  const [rows, setRows] = useState<RmmHost[]>([]);
  const [currentRmmVersion, setCurrentRmmVersion] = useState<string>("");
  const [error, setError] = useState<string | null>(null);
  const [msg, setMsg] = useState<string | null>(null);
  const [label, setLabel] = useState("");
  const [nameHint, setNameHint] = useState("");
  const [rdpUser, setRdpUser] = useState("fleetdeck");
  const [command, setCommand] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [busy, setBusy] = useState(false);

  const load = useCallback(() => {
    apiFetch("/api/v1/rmm/hosts")
      .then(async (r) => {
        const body = await r.json();
        if (!r.ok) throw new Error(body?.error?.message ?? "Could not load RMM hosts");
        setRows(body.data ?? []);
        if (typeof body.current_rmm_version === "string") {
          setCurrentRmmVersion(body.current_rmm_version);
        }
      })
      .catch((e) => setError(e instanceof Error ? e.message : "Failed"));
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
    if (type === "rmm.hosts.updated" || type === "rmm.session.updated") load();
  });

  async function createInstallCommand(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    setMsg(null);
    setCommand(null);
    setCopied(false);
    try {
      const res = await apiFetch("/api/v1/rmm/install-command", {
        method: "POST",
        body: JSON.stringify({
          label,
          name_hint: nameHint,
          rdp_user: rdpUser.trim() || "fleetdeck",
          ttl_minutes: 60,
          max_uses: 1,
        }),
      });
      const body = await res.json();
      if (!res.ok) throw new Error(body?.error?.message ?? "Could not create install command");
      setCommand(body.command);
      setMsg(
        body.note ??
          "Copy the command into elevated PowerShell on the Windows host. It enables RDP and creates a local user.",
      );
      setLabel("");
      setNameHint("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  async function copyCommand() {
    if (!command) return;
    await navigator.clipboard.writeText(command);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 2000);
  }

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <div>
        <h1 className="text-3xl font-semibold tracking-tight">Remote</h1>
        <p className="mt-1 text-sm text-[var(--text-1)]">
          Windows hosts with browser RDP. One elevated PowerShell command installs the agent, enables Remote
          Desktop, and creates a local RDP user.
        </p>
      </div>

      {error && (
        <div className="rounded-md border border-[var(--crit)]/40 bg-[var(--crit)]/10 px-3 py-2 text-sm text-[var(--crit)]">
          {error}
        </div>
      )}
      {msg && (
        <div className="rounded-md border border-[var(--border)] bg-[var(--bg-2)] px-3 py-2 text-sm text-[var(--text-1)]">
          {msg}
        </div>
      )}

      <form onSubmit={createInstallCommand} className="space-y-3 rounded-[var(--radius)] border border-[var(--border)] p-4">
        <div className="text-sm font-medium text-[var(--text-0)]">Install on a Windows PC</div>
        <p className="text-xs text-[var(--text-2)]">
          Run the generated command in <strong>Administrator</strong> PowerShell on the Windows host. The host dials
          out only — no inbound 3389 on the internet. Install creates a local RDP user and sends the password to
          FleetDeck (encrypted) so Connect auto-logs in.
        </p>
        <div className="flex flex-wrap gap-2">
          <input
            className="min-w-[10rem] flex-1 rounded-md border border-[var(--border)] bg-[var(--bg-0)] px-3 py-2 text-sm"
            placeholder="Label (optional)"
            value={label}
            onChange={(e) => setLabel(e.target.value)}
          />
          <input
            className="min-w-[10rem] flex-1 rounded-md border border-[var(--border)] bg-[var(--bg-0)] px-3 py-2 text-sm"
            placeholder="Name hint (optional)"
            value={nameHint}
            onChange={(e) => setNameHint(e.target.value)}
          />
          <input
            className="w-40 rounded-md border border-[var(--border)] bg-[var(--bg-0)] px-3 py-2 text-sm"
            placeholder="RDP username"
            value={rdpUser}
            onChange={(e) => setRdpUser(e.target.value)}
            title="Local Windows account the installer will create"
          />
          <button
            type="submit"
            disabled={busy}
            className="rounded-md bg-[var(--text-0)] px-3 py-2 text-sm text-[var(--bg-0)] disabled:opacity-50"
          >
            {busy ? "Generating…" : "Get install command"}
          </button>
        </div>
        {command && (
          <div className="space-y-2">
            <pre className="overflow-x-auto rounded-md bg-[var(--bg-2)] p-3 font-[family-name:var(--font-mono-family)] text-xs whitespace-pre-wrap break-all">
              {command}
            </pre>
            <button
              type="button"
              onClick={() => void copyCommand()}
              className="rounded-md border border-[var(--border)] px-3 py-1.5 text-xs"
            >
              {copied ? "Copied" : "Copy command"}
            </button>
          </div>
        )}
      </form>

      {rows.length === 0 ? (
        <div className="rounded-[var(--radius)] border border-dashed border-[var(--border-strong)] px-5 py-10 text-center text-sm text-[var(--text-2)]">
          No RMM hosts enrolled yet.
        </div>
      ) : (
        <div className="overflow-hidden rounded-[var(--radius)] border border-[var(--border)]">
          <table className="w-full text-left text-sm">
            <thead className="bg-[var(--bg-2)] text-[11px] uppercase tracking-[0.12em] text-[var(--text-2)]">
              <tr>
                <th className="px-4 py-3">Host</th>
                <th className="px-4 py-3">Agent</th>
                <th className="px-4 py-3">RDP</th>
                <th className="px-4 py-3">Status</th>
                <th className="px-4 py-3">Heartbeat</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((h) => (
                <tr key={h.id} className="border-t border-[var(--border)] bg-[var(--bg-1)]">
                  <td className="px-4 py-3">
                    <Link href={`/rmm/${h.id}`} className="text-[var(--text-0)] underline-offset-2 hover:underline">
                      {h.name}
                    </Link>
                    <div className="text-xs text-[var(--text-2)]">{h.hostname || "—"}</div>
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex flex-wrap items-center gap-2 font-[family-name:var(--font-mono-family)] text-xs">
                      <span>{h.agent_version || "—"}</span>
                      {agentVersionOutdated(h.agent_version, currentRmmVersion) && (
                        <Link href={`/rmm/${h.id}`} title="Open host to Update agent">
                          <StatusPill state="outdated" />
                        </Link>
                      )}
                    </div>
                  </td>
                  <td className="px-4 py-3 text-[var(--text-2)]">
                    {h.rdp_enabled ? `enabled :${h.rdp_port}` : "disabled"}
                  </td>
                  <td className="px-4 py-3">
                    <StatusPill
                      state={h.status === "online" ? "online" : h.status === "offline" ? "offline" : "pending"}
                    />
                  </td>
                  <td className="px-4 py-3 text-[var(--text-2)]">
                    {h.last_heartbeat_at ? new Date(h.last_heartbeat_at).toLocaleString() : "—"}
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
