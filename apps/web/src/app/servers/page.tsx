"use client";

import { FormEvent, useEffect, useState } from "react";
import Link from "next/link";
import { api, apiFetch, Server } from "@/lib/api";
import { StatusPill, agentVersionOutdated, agentSupportsPanelUpdate, AGENT_MANUAL_UPGRADE_CMD } from "@/components/status-pill";

function agentLooksOnline(s: Server): boolean {
  if (s.status !== "online") return false;
  if (!s.last_seen_at) return false;
  return Date.now() - new Date(s.last_seen_at).getTime() < 2 * 60 * 1000;
}

export default function ServersPage() {
  const [servers, setServers] = useState<Server[]>([]);
  const [currentAgentVersion, setCurrentAgentVersion] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [token, setToken] = useState<string | null>(null);
  const [cdnCommand, setCdnCommand] = useState<string | null>(null);
  const [apiUrl, setApiUrl] = useState<string | null>(null);
  const [bundleName, setBundleName] = useState<string | null>(null);
  const [bundleReady, setBundleReady] = useState(false);
  const [bundleBlob, setBundleBlob] = useState<Blob | null>(null);
  const [busyBuild, setBusyBuild] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [copied, setCopied] = useState(false);
  const [showOffline, setShowOffline] = useState(false);
  const [removingId, setRemovingId] = useState<string | null>(null);
  const [removeBusy, setRemoveBusy] = useState(false);
  const [removeMsg, setRemoveMsg] = useState<string | null>(null);
  const [updatingId, setUpdatingId] = useState<string | null>(null);
  const [updateBusy, setUpdateBusy] = useState(false);
  const [updateMsg, setUpdateMsg] = useState<string | null>(null);
  const [upgradeCopied, setUpgradeCopied] = useState(false);

  async function refresh() {
    const res = await api.servers();
    setServers(res.data);
    setCurrentAgentVersion(res.meta.current_agent_version ?? null);
  }

  useEffect(() => {
    let cancelled = false;
    const id = window.setTimeout(() => {
      api
        .servers()
        .then((res) => {
          if (!cancelled) {
            setServers(res.data);
            setCurrentAgentVersion(res.meta.current_agent_version ?? null);
          }
        })
        .catch((e) => {
          if (!cancelled) setError(e instanceof Error ? e.message : "Failed to load");
        });
    }, 0);
    return () => {
      cancelled = true;
      window.clearTimeout(id);
    };
  }, []);

  async function buildInstallers(enrollmentToken: string, serverName: string) {
    setBusyBuild(true);
    setCdnCommand(null);
    setApiUrl(null);
    setBundleReady(false);
    setBundleBlob(null);
    setError(null);
    try {
      const body = JSON.stringify({
        token: enrollmentToken,
        name: serverName,
      });

      const cmdRes = await apiFetch("/api/v1/agents/install-command", { method: "POST", body });
      if (!cmdRes.ok) {
        const data = await cmdRes.json().catch(() => null);
        throw new Error(data?.error?.message ?? "Could not build install command");
      }
      const cmdData = await cmdRes.json();
      setCdnCommand(cmdData.command as string);
      setApiUrl((cmdData.api_url as string) || null);

      const bundleRes = await apiFetch("/api/v1/agents/install-bundle", {
        method: "POST",
        body: JSON.stringify({
          token: enrollmentToken,
          name: serverName,
          arch: "amd64",
          api_url: cmdData.api_url,
        }),
      });
      if (bundleRes.ok) {
        const cd = bundleRes.headers.get("Content-Disposition") || "";
        const match = /filename=([^;]+)/i.exec(cd);
        setBundleName(match?.[1]?.replace(/"/g, "") || `fleetdeck-install-${serverName}-amd64.sh`);
        setBundleBlob(await bundleRes.blob());
        setBundleReady(true);
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "Could not build installer");
    } finally {
      setBusyBuild(false);
    }
  }

  async function onCreate(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    setToken(null);
    setCopied(false);
    setShowOffline(false);
    const serverName = name.trim();
    try {
      const created = await api.createServer(serverName);
      const tok = await api.createEnrollmentToken(created.id, serverName);
      setToken(tok.token);
      setName("");
      await refresh();
      await buildInstallers(tok.token, serverName);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not create server");
    } finally {
      setBusy(false);
    }
  }

  function downloadBundle() {
    if (!bundleBlob || !bundleName) return;
    const url = URL.createObjectURL(bundleBlob);
    const a = document.createElement("a");
    a.href = url;
    a.download = bundleName;
    a.click();
    URL.revokeObjectURL(url);
  }

  async function copyCommand(text: string | null) {
    if (!text) return;
    await navigator.clipboard.writeText(text);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 2000);
  }

  async function removeServer(s: Server, force: boolean) {
    setRemoveBusy(true);
    setRemoveMsg(null);
    setError(null);
    try {
      const res = await apiFetch(`/api/v1/servers/${s.id}/remove`, {
        method: "POST",
        body: JSON.stringify({ confirm: true, force }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        const msg = data?.error?.message ?? "Could not remove server";
        if (data?.error?.details?.force_allowed || data?.error?.details?.force_required) {
          setRemoveMsg(msg);
        } else {
          throw new Error(msg);
        }
        return;
      }
      setRemovingId(null);
      setRemoveMsg(`Removed ${data.name ?? s.name} (${data.mode ?? "ok"}).`);
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Could not remove server");
    } finally {
      setRemoveBusy(false);
    }
  }

  async function updateAgent(s: Server) {
    setUpdateBusy(true);
    setUpdateMsg(null);
    setError(null);
    try {
      const res = await apiFetch(`/api/v1/servers/${s.id}/update-agent`, {
        method: "POST",
        body: JSON.stringify({ confirm: true }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        const msg = data?.error?.message ?? "Could not update agent";
        if (/unsupported|unknown command|agent\.update/i.test(msg) || !agentSupportsPanelUpdate(s.agent_version)) {
          setUpdateMsg(
            `${msg} This agent is too old for panel update. On the VPS run:\n${AGENT_MANUAL_UPGRADE_CMD}`,
          );
          return;
        }
        throw new Error(msg);
      }
      setUpdatingId(null);
      setUpdateMsg(
        data.message ??
          `Update queued for ${data.name ?? s.name}. Version refreshes on next heartbeat.`,
      );
      await refresh();
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Could not update agent";
      if (/unsupported|unknown command|agent\.update/i.test(msg)) {
        setUpdateMsg(
          `${msg} This agent is too old for panel update. On the VPS run:\n${AGENT_MANUAL_UPGRADE_CMD}`,
        );
      } else {
        setError(msg);
      }
    } finally {
      setUpdateBusy(false);
    }
  }

  async function copyUpgradeCommand() {
    await navigator.clipboard.writeText(AGENT_MANUAL_UPGRADE_CMD);
    setUpgradeCopied(true);
    window.setTimeout(() => setUpgradeCopied(false), 2000);
  }

  const pendingRemove = servers.find((s) => s.id === removingId) ?? null;
  const pendingUpdate = servers.find((s) => s.id === updatingId) ?? null;

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <div>
        <div className="text-[11px] font-medium uppercase tracking-[0.18em] text-[var(--text-2)]">
          Fleet
        </div>
        <h1 className="mt-1 text-3xl font-semibold tracking-tight">Servers</h1>
        <p className="mt-1 text-sm text-[var(--text-1)]">
          Add a server, copy the one-liner, run it on the VPS. Binaries come from CDN; agents dial your
          API through Cloudflare Tunnel.
        </p>
      </div>

      <form
        onSubmit={onCreate}
        className="flex flex-col gap-3 rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-4 sm:flex-row"
      >
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Friendly name (e.g. web-01)"
          required
          className="min-w-0 flex-1 rounded-md border border-[var(--border)] bg-[var(--bg-2)] px-3 py-2 text-sm"
        />
        <button
          type="submit"
          disabled={busy || busyBuild}
          className="rounded-md bg-[var(--accent)] px-4 py-2 text-sm font-medium text-white disabled:opacity-60"
        >
          {busy || busyBuild ? "Working…" : "Add server"}
        </button>
      </form>

      {error && (
        <div className="rounded-md border border-[var(--crit)]/40 bg-[var(--crit)]/10 px-3 py-2 text-sm text-[var(--crit)]">
          {error}
        </div>
      )}
      {removeMsg && (
        <div className="rounded-md border border-[var(--border)] bg-[var(--bg-1)] px-3 py-2 text-sm text-[var(--text-1)]">
          {removeMsg}
        </div>
      )}
      {updateMsg && !pendingUpdate && (
        <div className="whitespace-pre-wrap rounded-md border border-[var(--border)] bg-[var(--bg-1)] px-3 py-2 text-sm text-[var(--text-1)]">
          {updateMsg}
        </div>
      )}

      {(token || cdnCommand || busyBuild) && (
        <section className="space-y-3 rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-4">
          <h2 className="text-sm font-medium">Install on VPS</h2>
          {token && (
            <p className="text-xs text-[var(--text-2)]">
              Enrollment token:{" "}
              <span className="font-[family-name:var(--font-mono-family)] text-[var(--text-1)]">{token}</span>
            </p>
          )}
          {apiUrl && (
            <p className="text-xs text-[var(--text-2)]">
              Agent API: <code className="text-[var(--text-1)]">{apiUrl}</code> (keep FleetDeck running on this PC)
            </p>
          )}
          {busyBuild && <p className="text-sm text-[var(--text-2)]">Building install command…</p>}
          {cdnCommand && (
            <>
              <pre className="overflow-auto rounded-md border border-[var(--border)] bg-[var(--bg-0)] p-3 font-[family-name:var(--font-mono-family)] text-xs break-all text-[var(--text-1)]">
                {cdnCommand}
              </pre>
              <button
                type="button"
                onClick={() => void copyCommand(cdnCommand)}
                className="rounded-md bg-[var(--accent)] px-3 py-1.5 text-xs text-white"
              >
                {copied ? "Copied" : "Copy install command"}
              </button>
            </>
          )}

          {bundleReady && (
            <div className="border-t border-[var(--border)] pt-3">
              <button
                type="button"
                className="text-xs text-[var(--text-2)] underline"
                onClick={() => setShowOffline((v) => !v)}
              >
                {showOffline ? "Hide air-gapped fallback" : "Air-gapped fallback (offline .sh)"}
              </button>
              {showOffline && bundleBlob && (
                <div className="mt-3 space-y-2">
                  <p className="text-sm text-[var(--text-1)]">
                    Download, scp to the VPS, then run. Still needs Cloudflare Tunnel for enroll/metrics.
                  </p>
                  <button
                    type="button"
                    onClick={downloadBundle}
                    className="rounded-md border border-[var(--border)] px-3 py-1.5 text-xs"
                  >
                    Download {bundleName}
                  </button>
                </div>
              )}
            </div>
          )}
        </section>
      )}

      {pendingUpdate && (
        <section className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-4">
          <h2 className="text-sm font-medium">Update agent on {pendingUpdate.name}?</h2>
          {agentLooksOnline(pendingUpdate) ? (
            agentSupportsPanelUpdate(pendingUpdate.agent_version) ? (
              <p className="mt-2 text-sm text-[var(--text-1)]">
                Pulls the current CDN channel binary, verifies SHA256 when available, replaces the agent
                binary, and restarts the service. Enrollment credentials stay on the host.
              </p>
            ) : (
              <div className="mt-2 space-y-2 text-sm text-[var(--text-1)]">
                <p>
                  Agent {pendingUpdate.agent_version || "unknown"} does not support panel Update
                  (needs 0.4.2+). Run this on the VPS — credentials are kept, no new token:
                </p>
                <pre className="overflow-x-auto rounded-md border border-[var(--border)] bg-[var(--bg-2)] px-3 py-2 font-[family-name:var(--font-mono-family)] text-xs">
                  {AGENT_MANUAL_UPGRADE_CMD}
                </pre>
                <button
                  type="button"
                  className="rounded-md border border-[var(--border)] px-3 py-1.5 text-xs"
                  onClick={() => void copyUpgradeCommand()}
                >
                  {upgradeCopied ? "Copied" : "Copy command"}
                </button>
              </div>
            )
          ) : (
            <p className="mt-2 text-sm text-[var(--text-1)]">
              Agent appears offline. Wait until it is online, then retry — or run the manual upgrade
              one-liner on the host if the agent is too old for panel update.
            </p>
          )}
          {updateMsg && (
            <pre className="mt-2 whitespace-pre-wrap text-sm text-[var(--crit)]">{updateMsg}</pre>
          )}
          <div className="mt-3 flex flex-wrap gap-2">
            {agentLooksOnline(pendingUpdate) && agentSupportsPanelUpdate(pendingUpdate.agent_version) && (
              <button
                type="button"
                disabled={updateBusy}
                className="rounded-md bg-[var(--accent)] px-3 py-1.5 text-xs text-white disabled:opacity-60"
                onClick={() => void updateAgent(pendingUpdate)}
              >
                {updateBusy ? "Updating…" : "Update agent"}
              </button>
            )}
            {agentLooksOnline(pendingUpdate) && !agentSupportsPanelUpdate(pendingUpdate.agent_version) && (
              <button
                type="button"
                disabled={updateBusy}
                className="rounded-md border border-[var(--border)] px-3 py-1.5 text-xs disabled:opacity-60"
                onClick={() => void updateAgent(pendingUpdate)}
              >
                {updateBusy ? "Trying…" : "Try panel update anyway"}
              </button>
            )}
            <button
              type="button"
              disabled={updateBusy}
              className="rounded-md border border-[var(--border)] px-3 py-1.5 text-xs"
              onClick={() => {
                setUpdatingId(null);
                setUpdateMsg(null);
              }}
            >
              {agentLooksOnline(pendingUpdate) ? "Cancel" : "Dismiss"}
            </button>
          </div>
        </section>
      )}

      {pendingRemove && (
        <section className="rounded-[var(--radius)] border border-[var(--crit)]/40 bg-[var(--crit)]/10 p-4">
          <h2 className="text-sm font-medium text-[var(--crit)]">Remove {pendingRemove.name}?</h2>
          {agentLooksOnline(pendingRemove) ? (
            <p className="mt-2 text-sm text-[var(--text-1)]">
              The agent is online. FleetDeck will ask it to uninstall (stop systemd unit, remove binary and
              state), then delete panel records.
            </p>
          ) : pendingRemove.status === "pending" ? (
            <p className="mt-2 text-sm text-[var(--text-1)]">
              This server was never enrolled. Remove deletes panel records only.
            </p>
          ) : (
            <p className="mt-2 text-sm text-[var(--text-1)]">
              Agent appears offline. You can only remove it from the panel (the VPS agent will not be
              uninstalled). Use force remove, or uninstall manually on the host with{" "}
              <code className="text-xs">sudo fleetdeck-agent -uninstall</code>.
            </p>
          )}
          <div className="mt-3 flex flex-wrap gap-2">
            {agentLooksOnline(pendingRemove) || pendingRemove.status === "pending" ? (
              <button
                type="button"
                disabled={removeBusy}
                className="rounded-md bg-[var(--crit)] px-3 py-1.5 text-xs text-white disabled:opacity-60"
                onClick={() => void removeServer(pendingRemove, false)}
              >
                {removeBusy
                  ? "Removing…"
                  : agentLooksOnline(pendingRemove)
                    ? "Uninstall & remove"
                    : "Remove"}
              </button>
            ) : null}
            <button
              type="button"
              disabled={removeBusy}
              className="rounded-md border border-[var(--crit)]/50 px-3 py-1.5 text-xs text-[var(--crit)] disabled:opacity-60"
              onClick={() => void removeServer(pendingRemove, true)}
            >
              {removeBusy ? "Working…" : "Force remove from panel"}
            </button>
            <button
              type="button"
              disabled={removeBusy}
              className="rounded-md border border-[var(--border)] px-3 py-1.5 text-xs"
              onClick={() => {
                setRemovingId(null);
                setRemoveMsg(null);
              }}
            >
              Cancel
            </button>
          </div>
        </section>
      )}

      {servers.length === 0 ? (
        <div className="rounded-[var(--radius)] border border-dashed border-[var(--border-strong)] px-6 py-12 text-center text-sm text-[var(--text-1)]">
          No servers yet. Add one to get an install command.
        </div>
      ) : (
        <div className="overflow-hidden rounded-[var(--radius)] border border-[var(--border)]">
          <table className="w-full text-left text-sm">
            <thead className="bg-[var(--bg-2)] text-[11px] uppercase tracking-[0.12em] text-[var(--text-2)]">
              <tr>
                <th className="px-4 py-3 font-medium">Server</th>
                <th className="px-4 py-3 font-medium">Status</th>
                <th className="px-4 py-3 font-medium">OS</th>
                <th className="px-4 py-3 font-medium">Agent</th>
                <th className="px-4 py-3 font-medium">Last seen</th>
                <th className="px-4 py-3 font-medium"> </th>
              </tr>
            </thead>
            <tbody>
              {servers.map((s) => (
                <tr key={s.id} className="border-t border-[var(--border)] bg-[var(--bg-1)]">
                  <td className="px-4 py-3">
                    <Link href={`/servers/${s.id}`} className="font-medium text-[var(--accent)] hover:underline">
                      {s.name}
                    </Link>
                    <div className="text-xs text-[var(--text-2)]">{s.hostname || s.id}</div>
                  </td>
                  <td className="px-4 py-3">
                    <StatusPill state={s.health_state || s.status} />
                  </td>
                  <td className="px-4 py-3 text-[var(--text-1)]">
                    {[s.os_name, s.os_version, s.arch].filter(Boolean).join(" · ") || "—"}
                  </td>
                  <td className="px-4 py-3 font-[family-name:var(--font-mono-family)] text-xs text-[var(--text-2)]">
                    <div className="flex flex-wrap items-center gap-2">
                      <span>{s.agent_version || "not enrolled"}</span>
                      {agentVersionOutdated(s.agent_version, currentAgentVersion) && (
                        <button
                          type="button"
                          className="inline-flex"
                          title={
                            currentAgentVersion
                              ? `Expected ${currentAgentVersion}. Open Update agent.`
                              : "Open Update agent"
                          }
                          disabled={updateBusy || removeBusy}
                          onClick={() => {
                            setUpdatingId(s.id);
                            setUpdateMsg(null);
                            setRemovingId(null);
                          }}
                        >
                          <StatusPill state="outdated" />
                        </button>
                      )}
                    </div>
                  </td>
                  <td className="px-4 py-3 text-[var(--text-2)]">
                    {s.last_seen_at ? new Date(s.last_seen_at).toLocaleString() : "—"}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <div className="flex flex-wrap justify-end gap-3">
                      {s.agent_version && (
                        <button
                          type="button"
                          className="text-xs text-[var(--accent)] hover:underline disabled:opacity-50"
                          disabled={updateBusy || removeBusy}
                          onClick={() => {
                            setUpdatingId(s.id);
                            setUpdateMsg(null);
                            setRemovingId(null);
                          }}
                        >
                          Update agent
                        </button>
                      )}
                      <button
                        type="button"
                        className="text-xs text-[var(--crit)] hover:underline disabled:opacity-50"
                        disabled={removeBusy || updateBusy}
                        onClick={() => {
                          setRemovingId(s.id);
                          setRemoveMsg(null);
                          setUpdatingId(null);
                        }}
                      >
                        Remove
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
