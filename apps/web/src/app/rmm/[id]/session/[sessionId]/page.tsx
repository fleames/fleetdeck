"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useCallback, useEffect, useRef, useState } from "react";
import { API_URL, apiFetch, ensureCsrfToken, getCsrfToken } from "@/lib/api";
import { useRealtime } from "@/lib/realtime";

function guacWsURL(sessionId: string): string {
  const explicit = process.env.NEXT_PUBLIC_WS_URL || (API_URL || undefined);
  if (explicit) {
    return explicit.replace(/^http/, "ws") + `/api/v1/rmm/sessions/${sessionId}/guac`;
  }
  if (typeof window === "undefined") {
    return `ws://127.0.0.1:8080/api/v1/rmm/sessions/${sessionId}/guac`;
  }
  const proto = window.location.protocol === "https:" ? "wss" : "ws";
  const host = window.location.hostname;
  return `${proto}://${host}:8080/api/v1/rmm/sessions/${sessionId}/guac`;
}

type ConnInfo = {
  hostname: string;
  port: number;
  security?: string;
  "ignore-cert"?: boolean;
  username?: string;
  auto_login?: boolean;
  rdp_concurrent_sessions?: boolean;
  rdp_session_note?: string;
};

function describeRDPViewerError(raw: string | undefined | null, autoLogin: boolean): string {
  const msg = (raw || "").trim();
  const lower = msg.toLowerCase();
  if (lower.includes("credentials expired")) {
    return (
      "Windows rejected the RDP login as credentials expired (password must be changed, or the local account still has “must change password at next logon”). " +
      "On the host, run: net user fleetdeck /logonpasswordchg:no — or paste the password from C:\\ProgramData\\FleetDeck\\RMM\\rdp-credentials.txt below and retry. " +
      "Restarting FleetDeckRMM (agent ≥ 0.1.3-dev) also clears that flag."
    );
  }
  if (
    lower.includes("authentication failure") ||
    lower.includes("invalid credentials") ||
    lower.includes("wrong password")
  ) {
    return (
      "Invalid RDP password (does not match the Windows local account). " +
      "Paste the current password from C:\\ProgramData\\FleetDeck\\RMM\\rdp-credentials.txt and retry, or re-run the host installer so the panel secret is rewritten."
    );
  }
  if (!autoLogin && (!msg || lower.includes("disconnected"))) {
    return (
      "No RDP password is stored for this host. Paste the password from C:\\ProgramData\\FleetDeck\\RMM\\rdp-credentials.txt (username .\\fleetdeck or fleetdeck) and retry."
    );
  }
  if (msg) return msg;
  return (
    "Tunnel is up but the desktop did not appear. This is usually an RDP login problem — retry below, or re-run the host installer so the panel secret matches the Windows account."
  );
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type GuacClient = any;

function fitDisplay(client: GuacClient, mount: HTMLDivElement | null) {
  if (!mount) return;
  try {
    const disp = client.getDisplay();
    const w = disp.getWidth();
    const h = disp.getHeight();
    if (w <= 0 || h <= 0) return;
    const scale = Math.min(mount.clientWidth / w, mount.clientHeight / h, 1);
    if (scale > 0 && Number.isFinite(scale)) disp.scale(scale);
  } catch {
    /* ignore */
  }
}

function nudgeDesktop(client: GuacClient) {
  try {
    // Wake locked / idle Windows sessions that render black until input arrives.
    client.sendMouseState({ x: 1, y: 1, left: false, middle: false, right: false, up: false, down: false });
    client.sendMouseState({ x: 40, y: 40, left: false, middle: false, right: false, up: false, down: false });
  } catch {
    /* ignore */
  }
}

export default function RmmSessionPage() {
  const params = useParams<{ id: string; sessionId: string }>();
  const router = useRouter();
  const hostId = params.id;
  const sessionId = params.sessionId;
  const displayRef = useRef<HTMLDivElement>(null);
  const clientRef = useRef<{ disconnect: () => void } | null>(null);
  const startedRef = useRef(false);
  const closedServerRef = useRef(false);
  const [status, setStatus] = useState("pending");
  const [error, setError] = useState<string | null>(null);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [started, setStarted] = useState(false);
  const [connected, setConnected] = useState(false);
  const [busy, setBusy] = useState(false);
  const [phase, setPhase] = useState("Waiting for agent tunnel…");
  const [sessionNote, setSessionNote] = useState<string | null>(null);
  const [concurrentOk, setConcurrentOk] = useState<boolean | null>(null);

  async function postDisconnect(keepalive = false) {
    if (closedServerRef.current) return;
    closedServerRef.current = true;
    const csrf = (await ensureCsrfToken()) || getCsrfToken();
    const headers: Record<string, string> = { "Content-Type": "application/json" };
    if (csrf) headers["X-CSRF-Token"] = csrf;
    try {
      await fetch(`${API_URL}/api/v1/rmm/sessions/${sessionId}/disconnect`, {
        method: "POST",
        credentials: "include",
        headers,
        body: JSON.stringify({ confirm: true }),
        keepalive,
        cache: "no-store",
      });
    } catch {
      closedServerRef.current = false;
    }
  }
  const [autoLogin, setAutoLogin] = useState(false);
  const [needsPassword, setNeedsPassword] = useState(false);

  const refreshSession = useCallback(() => {
    apiFetch(`/api/v1/rmm/sessions/${sessionId}`)
      .then(async (r) => {
        const body = await r.json();
        if (!r.ok) throw new Error(body?.error?.message ?? "Session missing");
        setStatus(body.status);
      })
      .catch((e) => setError(e instanceof Error ? e.message : "Failed"));
  }, [sessionId]);

  useEffect(() => {
    const boot = window.setTimeout(refreshSession, 0);
    const poll = window.setInterval(refreshSession, 3000);
    return () => {
      window.clearTimeout(boot);
      window.clearInterval(poll);
    };
  }, [refreshSession]);

  useRealtime((type) => {
    if (type === "rmm.session.updated") refreshSession();
  });

  const startViewer = useCallback(
    async (overrideUser?: string, overridePass?: string) => {
      if (startedRef.current) return;
      startedRef.current = true;
      setBusy(true);
      setError(null);
      setConnected(false);
      setPhase("Waiting for agent tunnel…");
      try {
        let info: ConnInfo | null = null;
        for (let i = 0; i < 40; i++) {
          const res = await apiFetch(`/api/v1/rmm/sessions/${sessionId}/connection`);
          const body = await res.json();
          if (res.ok) {
            info = body;
            setStatus("active");
            break;
          }
          setPhase(`Waiting for agent tunnel… (${i + 1})`);
          await new Promise((r) => setTimeout(r, 500));
        }
        if (!info) {
          throw new Error("Agent tunnel not ready. Is the RMM agent online?");
        }
        if (typeof info.rdp_concurrent_sessions === "boolean") {
          setConcurrentOk(info.rdp_concurrent_sessions);
        }
        if (info.rdp_session_note) {
          setSessionNote(info.rdp_session_note);
        }

        const user = overrideUser || info.username || username;
        const pass = overridePass || password;
        const manualOverride = Boolean(overrideUser || overridePass);
        let canAuto = Boolean(info.auto_login);
        setAutoLogin(canAuto);
        if (info.username) setUsername(info.username);

        if (!canAuto && !pass && !manualOverride) {
          setNeedsPassword(true);
          setError(describeRDPViewerError(null, false));
          setPhase("Password required");
          startedRef.current = false;
          setBusy(false);
          return;
        }
        setNeedsPassword(false);

        // Push pasted credentials over HTTPS JSON — never put passwords in the Guacamole WS URL.
        if (manualOverride && pass) {
          const credRes = await apiFetch(`/api/v1/rmm/sessions/${sessionId}/rdp-credentials`, {
            method: "POST",
            body: JSON.stringify({
              username: user || info.username || "fleetdeck",
              password: pass,
            }),
          });
          const credBody = await credRes.json();
          if (!credRes.ok) {
            throw new Error(credBody?.error?.message ?? "Could not store RDP credentials");
          }
          canAuto = true;
          setAutoLogin(true);
          if (credBody?.username) setUsername(credBody.username);
        }

        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const GuacamoleMod: any = await import("guacamole-common-js");
        const Guacamole = GuacamoleMod.default ?? GuacamoleMod;
        const tunnel = new Guacamole.WebSocketTunnel(guacWsURL(sessionId));
        const client = new Guacamole.Client(tunnel);
        clientRef.current = client;

        const display = client.getDisplay().getElement();
        display.style.display = "block";
        display.style.maxWidth = "100%";
        const mount = displayRef.current;
        if (mount) {
          mount.innerHTML = "";
          mount.appendChild(display);
        }

        client.onerror = (statusObj: { message?: string; code?: number }) => {
          const msg = describeRDPViewerError(statusObj?.message || `Guacamole error ${statusObj?.code ?? ""}`, canAuto);
          setError(msg);
          setPhase("Failed");
          setConnected(false);
          startedRef.current = false;
          // Server also closes on guac error; ensure panel slot is freed if WS dies first.
          void postDisconnect();
          setStatus("closed");
        };

        // Display params only — hostname/port/credentials are forced server-side.
        const params = new URLSearchParams({
          security: info.security || "any",
          "ignore-cert": "true",
          "enable-wallpaper": "true",
          "resize-method": "display-update",
          width: String(Math.max(1024, mount?.clientWidth || 1280)),
          height: String(Math.max(720, mount?.clientHeight || 800)),
          dpi: "96",
          "color-depth": "32",
        });

        setPhase(canAuto || pass ? "Connecting…" : "Connecting (enter credentials if prompted)…");

        client.onstatechange = (state: number) => {
          // 0 IDLE, 1 CONNECTING, 2 WAITING, 3 CONNECTED, 4 DISCONNECTED
          if (state === 3) {
            setPhase("Connected");
            setStatus("active");
            setConnected(true);
            fitDisplay(client, displayRef.current);
            nudgeDesktop(client);
            window.setTimeout(() => {
              fitDisplay(client, displayRef.current);
              nudgeDesktop(client);
            }, 500);
          } else if (state === 4) {
            setConnected(false);
            setPhase("Disconnected");
          }
        };

        client.connect(params.toString());
        setStarted(true);

        const sink = new Guacamole.Keyboard(document);
        sink.onkeydown = (keysym: number) => client.sendKeyEvent(1, keysym);
        sink.onkeyup = (keysym: number) => client.sendKeyEvent(0, keysym);

        const mouse = new Guacamole.Mouse(display);
        mouse.onmousedown = mouse.onmouseup = mouse.onmousemove = (state: unknown) => {
          client.sendMouseState(state);
        };

        const onResize = () => fitDisplay(client, displayRef.current);
        window.addEventListener("resize", onResize);
        // Stash remover on disconnect path via client wrapper.
        const prevDisconnect = client.disconnect.bind(client);
        client.disconnect = () => {
          window.removeEventListener("resize", onResize);
          prevDisconnect();
        };
      } catch (err) {
        startedRef.current = false;
        setError(err instanceof Error ? err.message : "Could not start viewer");
      } finally {
        setBusy(false);
      }
    },
    [password, sessionId, username],
  );

  // Auto-connect while session is open; connection endpoint gates on tunnel attach.
  useEffect(() => {
    if (startedRef.current) return;
    if (status === "closed" || status === "failed") return;
    if (status !== "active" && status !== "pending" && status !== "connecting") return;
    const t = window.setTimeout(() => {
      let overrideUser: string | undefined;
      let overridePass: string | undefined;
      try {
        const raw = sessionStorage.getItem(`rmm_retry_${sessionId}`);
        if (raw) {
          sessionStorage.removeItem(`rmm_retry_${sessionId}`);
          const parsed = JSON.parse(raw) as { username?: string; password?: string };
          if (parsed.username) {
            overrideUser = parsed.username;
            setUsername(parsed.username);
          }
          if (parsed.password) {
            overridePass = parsed.password;
            setPassword(parsed.password);
          }
        }
      } catch {
        /* ignore */
      }
      void startViewer(overrideUser, overridePass);
    }, 400);
    return () => {
      window.clearTimeout(t);
    };
  }, [startViewer, status, sessionId]);

  useEffect(() => {
    return () => {
      clientRef.current?.disconnect();
      clientRef.current = null;
      // Allow reconnect after React Strict Mode remount / navigation.
      startedRef.current = false;
    };
  }, []);

  // Mark session closed when leaving the page (back nav / tab close) so orphans are rare.
  useEffect(() => {
    const onLeave = () => {
      void postDisconnect(true);
    };
    window.addEventListener("pagehide", onLeave);
    window.addEventListener("beforeunload", onLeave);
    return () => {
      window.removeEventListener("pagehide", onLeave);
      window.removeEventListener("beforeunload", onLeave);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- sessionId stable for this page mount
  }, [sessionId]);

  async function disconnect() {
    clientRef.current?.disconnect();
    clientRef.current = null;
    startedRef.current = false;
    setConnected(false);
    await postDisconnect();
    setStatus("closed");
  }

  async function retryConnect() {
    clientRef.current?.disconnect();
    clientRef.current = null;
    startedRef.current = false;
    setStarted(false);
    setConnected(false);
    // Auth failure / unload closes the server session — start a fresh one (supersedes leftovers).
    if (closedServerRef.current || status === "closed" || status === "failed") {
      setBusy(true);
      setError(null);
      try {
        closedServerRef.current = false;
        const res = await apiFetch(`/api/v1/rmm/hosts/${hostId}/sessions`, {
          method: "POST",
          body: JSON.stringify({ confirm: true }),
        });
        const body = await res.json();
        if (!res.ok) throw new Error(body?.error?.message ?? "Could not start session");
        if (username.trim() || password.trim()) {
          try {
            sessionStorage.setItem(
              `rmm_retry_${body.id}`,
              JSON.stringify({ username: username.trim(), password }),
            );
          } catch {
            /* ignore quota / private mode */
          }
        }
        router.replace(`/rmm/${hostId}/session/${body.id}`);
      } catch (err) {
        setError(err instanceof Error ? err.message : "Could not start session");
        setBusy(false);
      }
      return;
    }
    void startViewer(username, password);
  }

  return (
    <div className="mx-auto flex max-w-6xl flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <Link href={`/rmm/${hostId}`} className="text-xs text-[var(--text-2)] hover:text-[var(--text-0)]">
            ← Host
          </Link>
          <h1 className="mt-1 text-2xl font-semibold tracking-tight">Browser RDP</h1>
          <p className="text-xs text-[var(--text-2)]">
            Session {sessionId.slice(0, 8)}… · status {status}
            {connected ? " · desktop connected" : busy || !started ? ` · ${phase}` : ` · ${phase}`}
          </p>
        </div>
        <button
          type="button"
          onClick={disconnect}
          className="rounded-md border border-[var(--border)] px-3 py-2 text-sm"
        >
          Disconnect
        </button>
      </div>

      {sessionNote && concurrentOk === false && (
        <div className="rounded-md border border-[var(--border)] bg-[var(--bg-1)] px-3 py-2 text-sm text-[var(--text-1)]">
          {sessionNote}
        </div>
      )}

      {(error ||
        needsPassword ||
        (started && !connected && !busy && phase !== "Connecting…" && phase !== "Waiting for agent tunnel…")) && (
        <div className="space-y-2 rounded-md border border-[var(--crit)]/40 bg-[var(--crit)]/10 px-3 py-2 text-sm text-[var(--crit)]">
          <div>
            {error ||
              describeRDPViewerError(null, autoLogin)}
          </div>
          <div className="flex flex-wrap gap-2">
            <input
              className="rounded-md border border-[var(--border)] bg-[var(--bg-0)] px-3 py-2 text-sm text-[var(--text-0)]"
              placeholder="Username (e.g. .\\fleetdeck)"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
            />
            <input
              type="password"
              className="rounded-md border border-[var(--border)] bg-[var(--bg-0)] px-3 py-2 text-sm text-[var(--text-0)]"
              placeholder={needsPassword || !autoLogin ? "Paste password from rdp-credentials.txt" : "Password"}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="off"
            />
            <button
              type="button"
              disabled={busy || ((needsPassword || !autoLogin) && !password.trim())}
              onClick={() => void retryConnect()}
              className="rounded-md bg-[var(--text-0)] px-3 py-2 text-sm text-[var(--bg-0)] disabled:opacity-50"
            >
              Retry connect
            </button>
          </div>
        </div>
      )}

      <div
        ref={displayRef}
        className="relative min-h-[480px] w-full overflow-auto rounded-[var(--radius)] border border-[var(--border)] bg-black"
      />
    </div>
  );
}
