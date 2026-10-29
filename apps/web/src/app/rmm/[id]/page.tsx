"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { apiFetch } from "@/lib/api";
import { agentVersionOutdated } from "@/lib/agent-version";
import { StatusPill } from "@/components/status-pill";
import { useRealtime } from "@/lib/realtime";

type OpenSession = {
  id: string;
  mode: string;
  status: string;
  created_at: string;
  viewer_url?: string;
};

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
  rdp_username?: string;
  has_rdp_credentials?: boolean;
  os_edition?: string;
  os_product_type?: string;
  rdp_concurrent_sessions?: boolean;
  rdp_session_note?: string;
  last_heartbeat_at: string | null;
  current_rmm_version?: string;
  open_session?: OpenSession | null;
};

export default function RmmHostPage() {
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const id = params.id;
  const [host, setHost] = useState<RmmHost | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [confirmUpdate, setConfirmUpdate] = useState(false);
  const [updateBusy, setUpdateBusy] = useState(false);
  const [updateMsg, setUpdateMsg] = useState<string | null>(null);
  const [nativeOut, setNativeOut] = useState<{
    token: string;
    gateway_host: string;
    gateway_port: number;
    rdp: string;
    launcher_ps1: string;
    note: string;
  } | null>(null);

  const load = useCallback(() => {
    apiFetch(`/api/v1/rmm/hosts/${id}`)
      .then(async (r) => {
        const body = await r.json();
        if (!r.ok) throw new Error(body?.error?.message ?? "Could not load host");
        setHost(body);
      })
      .catch((e) => setError(e instanceof Error ? e.message : "Failed"));
  }, [id]);

  useEffect(() => {
    const boot = window.setTimeout(load, 0);
    return () => window.clearTimeout(boot);
  }, [load]);

  useRealtime((type) => {
    if (type === "rmm.hosts.updated" || type === "rmm.session.updated") load();
  });

  async function connectBrowser() {
    if (!confirm("Start a browser RDP session to this host?")) return;
    setBusy(true);
    setError(null);
    try {
      const res = await apiFetch(`/api/v1/rmm/hosts/${id}/sessions`, {
        method: "POST",
        body: JSON.stringify({ confirm: true }),
      });
      const body = await res.json();
      if (!res.ok) throw new Error(body?.error?.message ?? "Could not start session");
      router.push(`/rmm/${id}/session/${body.id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  async function disconnectOpenSession() {
    const sess = host?.open_session;
    if (!sess) return;
    if (!confirm("Disconnect the active RDP session on this host?")) return;
    setBusy(true);
    setError(null);
    try {
      const res = await apiFetch(`/api/v1/rmm/sessions/${sess.id}/disconnect`, {
        method: "POST",
        body: JSON.stringify({ confirm: true }),
      });
      const body = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(body?.error?.message ?? "Could not disconnect");
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  async function connectNative() {
    if (!confirm("Create a short-lived native Remote Desktop session?")) return;
    setBusy(true);
    setError(null);
    setNativeOut(null);
    try {
      const res = await apiFetch(`/api/v1/rmm/hosts/${id}/native-session`, {
        method: "POST",
        body: JSON.stringify({ confirm: true }),
      });
      const body = await res.json();
      if (!res.ok) throw new Error(body?.error?.message ?? "Could not start native session");
      setNativeOut(body);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  async function updateAgent() {
    if (!host) return;
    setUpdateBusy(true);
    setUpdateMsg(null);
    try {
      const res = await apiFetch(`/api/v1/rmm/hosts/${host.id}/update-agent`, {
        method: "POST",
        body: JSON.stringify({ confirm: true }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        const msg = data?.error?.message ?? "Could not update RMM agent";
        if (/unknown command/i.test(msg)) {
          setUpdateMsg(
            `${msg} This host needs RMM agent 0.1.1+. Re-run a panel install command (it will replace the binary and keep credentials), then use Update agent next time.`,
          );
          return;
        }
        setUpdateMsg(msg);
        return;
      }
      setConfirmUpdate(false);
      setUpdateMsg(
        data.message ?? "Update staged; agent is restarting. Version refreshes on next heartbeat.",
      );
      window.setTimeout(load, 4000);
    } catch (e) {
      setUpdateMsg(e instanceof Error ? e.message : "Could not update RMM agent");
    } finally {
      setUpdateBusy(false);
    }
  }

  async function removeHost() {
    if (!confirm("Remove this RMM host from FleetDeck? The Windows service will not be uninstalled automatically.")) return;
    setBusy(true);
    try {
      const res = await apiFetch(`/api/v1/rmm/hosts/${id}/remove`, {
        method: "POST",
        body: JSON.stringify({ confirm: true }),
      });
      const body = await res.json();
      if (!res.ok) throw new Error(body?.error?.message ?? "Remove failed");
      router.push("/rmm");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed");
    } finally {
      setBusy(false);
    }
  }

  function downloadText(filename: string, text: string) {
    const blob = new Blob([text], { type: "text/plain" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = filename;
    a.click();
    URL.revokeObjectURL(url);
  }

  if (!host && !error) {
    return <div className="text-sm text-[var(--text-2)]">Loading…</div>;
  }

  const online = host?.status === "online";
  const outdated = agentVersionOutdated(host?.agent_version, host?.current_rmm_version);
  const openSession = host?.open_session ?? null;

  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <Link href="/rmm" className="text-xs text-[var(--text-2)] hover:text-[var(--text-0)]">
            ← Remote
          </Link>
          <h1 className="mt-2 text-3xl font-semibold tracking-tight">{host?.name ?? "Host"}</h1>
          <p className="mt-1 text-sm text-[var(--text-1)]">
            {host?.hostname} · {host?.os_version || host?.os_name} · {host?.arch}
          </p>
        </div>
        <div className="flex items-center gap-2">
          {outdated && (
            <button
              type="button"
              className="inline-flex"
              title={
                host?.current_rmm_version
                  ? `Expected ${host.current_rmm_version}. Open Update agent.`
                  : "Open Update agent"
              }
              onClick={() => {
                setConfirmUpdate(true);
                setUpdateMsg(null);
              }}
            >
              <StatusPill state="outdated" />
            </button>
          )}
        </div>
      </div>

      {error && (
        <div className="rounded-md border border-[var(--crit)]/40 bg-[var(--crit)]/10 px-3 py-2 text-sm text-[var(--crit)]">
          {error}
        </div>
      )}

      {updateMsg && !confirmUpdate && (
        <div className="rounded-md border border-[var(--border)] bg-[var(--bg-1)] px-3 py-2 text-sm text-[var(--text-1)]">
          {updateMsg}
        </div>
      )}

      {host && (
        <div className="space-y-3 rounded-[var(--radius)] border border-[var(--border)] p-4 text-sm">
          <div className="flex items-center justify-between gap-3">
            <span className="text-[var(--text-2)]">Status</span>
            <StatusPill
              state={host.status === "online" ? "online" : host.status === "offline" ? "offline" : "pending"}
            />
          </div>
          <div className="flex items-center justify-between gap-3">
            <span className="text-[var(--text-2)]">Agent</span>
            <span className="font-[family-name:var(--font-mono-family)] text-xs">
              {host.agent_version || "—"}
              {host.current_rmm_version ? ` (expected ${host.current_rmm_version})` : ""}
            </span>
          </div>
          <div className="flex items-center justify-between gap-3">
            <span className="text-[var(--text-2)]">RDP</span>
            <span>{host.rdp_enabled ? `listening :${host.rdp_port}` : "not detected"}</span>
          </div>
          <div className="flex items-center justify-between gap-3">
            <span className="text-[var(--text-2)]">Auto-connect</span>
            <span>
              {host.has_rdp_credentials
                ? `ready (${host.rdp_username || "user"})`
                : host.rdp_username
                  ? `${host.rdp_username} (password missing)`
                  : "not configured"}
            </span>
          </div>
          <div className="flex items-center justify-between gap-3">
            <span className="text-[var(--text-2)]">Concurrent RDP</span>
            <span>
              {host.rdp_concurrent_sessions
                ? "supported (Windows Server)"
                : host.os_product_type === "server" || host.os_product_type === "domain_controller"
                  ? "supported (Windows Server)"
                  : "not on this SKU (Pro/client)"}
            </span>
          </div>
          <div className="flex items-center justify-between gap-3">
            <span className="text-[var(--text-2)]">Last heartbeat</span>
            <span>{host.last_heartbeat_at ? new Date(host.last_heartbeat_at).toLocaleString() : "—"}</span>
          </div>
        </div>
      )}

      {host &&
        !host.rdp_concurrent_sessions &&
        host.os_product_type !== "server" &&
        host.os_product_type !== "domain_controller" &&
        !/\bserver\b/i.test(host.os_version || "") && (
        <div className="rounded-md border border-[var(--border)] bg-[var(--bg-1)] px-3 py-2 text-sm text-[var(--text-1)]">
          {host.rdp_session_note ||
            "Windows Pro/client allows only one interactive desktop. FleetDeck will not kick the local user — if someone is at the console, Windows may show takeover / consent prompts. Concurrent sessions without kicking require Windows Server."}
        </div>
      )}

      {openSession && (
        <div className="space-y-3 rounded-[var(--radius)] border border-[var(--border)] p-4 text-sm">
          <div className="font-medium">Open session</div>
          <p className="text-[var(--text-2)]">
            {openSession.mode} · {openSession.status}
            {openSession.created_at
              ? ` · started ${new Date(openSession.created_at).toLocaleString()}`
              : ""}
          </p>
          <div className="flex flex-wrap gap-2">
            {openSession.mode === "browser" && (
              <Link
                href={`/rmm/${id}/session/${openSession.id}`}
                className="rounded-md bg-[var(--text-0)] px-3 py-2 text-sm text-[var(--bg-0)]"
              >
                Open session
              </Link>
            )}
            <button
              type="button"
              disabled={busy}
              onClick={() => void disconnectOpenSession()}
              className="rounded-md border border-[var(--border)] px-3 py-2 text-sm disabled:opacity-50"
            >
              Disconnect
            </button>
          </div>
        </div>
      )}

      {confirmUpdate && host && (
        <section className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-4">
          <h2 className="text-sm font-medium">Update RMM agent on {host.name}?</h2>
          {online ? (
            <p className="mt-2 text-sm text-[var(--text-1)]">
              Current version {host.agent_version || "—"}. Pulls CDN channel binary, verifies SHA256, replaces{" "}
              <span className="font-[family-name:var(--font-mono-family)] text-xs">fleetdeck-rmm.exe</span>, and
              restarts the FleetDeckRMM service. Credentials are preserved.
            </p>
          ) : (
            <p className="mt-2 text-sm text-[var(--text-1)]">
              Agent offline — wait until online, then retry.
            </p>
          )}
          {updateMsg && <pre className="mt-2 whitespace-pre-wrap text-sm text-[var(--crit)]">{updateMsg}</pre>}
          <div className="mt-3 flex flex-wrap gap-2">
            {online && (
              <button
                type="button"
                disabled={updateBusy}
                className="rounded-md bg-[var(--accent)] px-3 py-1.5 text-xs text-white disabled:opacity-60"
                onClick={() => void updateAgent()}
              >
                {updateBusy ? "Updating…" : "Update agent"}
              </button>
            )}
            <button
              type="button"
              disabled={updateBusy}
              className="rounded-md border border-[var(--border)] px-3 py-1.5 text-xs"
              onClick={() => setConfirmUpdate(false)}
            >
              {online ? "Cancel" : "Dismiss"}
            </button>
          </div>
        </section>
      )}

      <div className="flex flex-wrap gap-2">
        <button
          type="button"
          disabled={busy || !online}
          onClick={connectBrowser}
          className="rounded-md bg-[var(--text-0)] px-3 py-2 text-sm text-[var(--bg-0)] disabled:opacity-50"
        >
          Connect (browser)
        </button>
        <button
          type="button"
          disabled={busy || !online}
          onClick={connectNative}
          className="rounded-md border border-[var(--border)] px-3 py-2 text-sm disabled:opacity-50"
        >
          Native Remote Desktop
        </button>
        <button
          type="button"
          disabled={busy || !host}
          onClick={() => {
            setConfirmUpdate(true);
            setUpdateMsg(null);
          }}
          className="rounded-md border border-[var(--border)] px-3 py-2 text-sm disabled:opacity-50"
        >
          Update agent
        </button>
        <button
          type="button"
          disabled={busy}
          onClick={removeHost}
          className="rounded-md border border-[var(--crit)]/40 px-3 py-2 text-sm text-[var(--crit)] disabled:opacity-50"
        >
          Remove
        </button>
      </div>

      {nativeOut && (
        <div className="space-y-3 rounded-[var(--radius)] border border-[var(--border)] p-4 text-sm">
          <div className="font-medium">Native session ready</div>
          <p className="text-[var(--text-2)]">{nativeOut.note}</p>
          <pre className="overflow-x-auto rounded-md bg-[var(--bg-2)] p-3 font-[family-name:var(--font-mono-family)] text-xs">
            fleetdeck-rmm -connect {nativeOut.gateway_host}:{nativeOut.gateway_port} -connect-token {nativeOut.token}
          </pre>
          <div className="flex flex-wrap gap-2">
            <button
              type="button"
              className="rounded-md border border-[var(--border)] px-3 py-1.5 text-xs"
              onClick={() => downloadText("fleetdeck-session.rdp", nativeOut.rdp)}
            >
              Download .rdp
            </button>
            <button
              type="button"
              className="rounded-md border border-[var(--border)] px-3 py-1.5 text-xs"
              onClick={() => downloadText("fleetdeck-rdp.ps1", nativeOut.launcher_ps1)}
            >
              Download launcher notes
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
