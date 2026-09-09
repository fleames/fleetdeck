"use client";

import { ReactNode, useEffect, useRef, useState } from "react";
import { Sidebar } from "@/components/sidebar";
import { api, AuthStatus, Overview } from "@/lib/api";
import { AuthGate } from "@/components/auth-gate";
import { CommandPalette } from "@/components/command-palette";
import { ThemeSync } from "@/components/theme-sync";
import { NotificationBell } from "@/components/notification-bell";

export function AppShell({ children }: { children: ReactNode }) {
  const [auth, setAuth] = useState<AuthStatus | null>(null);
  const [overview, setOverview] = useState<Overview | null>(null);
  const [error, setError] = useState<string | null>(null);
  const knownAuthed = useRef(false);

  async function refresh(opts?: { allowCookieSettle?: boolean }) {
    try {
      let status = await api.authStatus();
      // After login/bootstrap, some browsers need a tick before the session cookie
      // is visible on the next same-origin request.
      if (opts?.allowCookieSettle && !status.authenticated && !status.bootstrap_required) {
        await new Promise((r) => window.setTimeout(r, 50));
        status = await api.authStatus();
      }
      // Never demote a known session on a single unauthenticated probe (cookie race /
      // transient proxy glitch). Explicit logout clears knownAuthed via onDone path.
      if (!status.authenticated && knownAuthed.current && !status.bootstrap_required) {
        await new Promise((r) => window.setTimeout(r, 100));
        const retry = await api.authStatus();
        if (retry.authenticated) status = retry;
        else {
          // Confirmed signed out.
          knownAuthed.current = false;
          setAuth(status);
          setError(null);
          return;
        }
      }
      if (status.authenticated) knownAuthed.current = true;
      setAuth(status);
      if (status.authenticated && !status.bootstrap_required) {
        try {
          const ov = await api.overview();
          setOverview(ov);
        } catch {
          // Keep prior overview on transient failure; session itself is fine.
        }
      }
      setError(null);
    } catch (e) {
      // Transient /auth/status failures (network, 503) must not clear a known session.
      setError(e instanceof Error ? e.message : "API unavailable");
    }
  }

  useEffect(() => {
    let cancelled = false;
    const id = window.setTimeout(() => {
      refresh()
        .catch((e) => {
          if (!cancelled) setError(e instanceof Error ? e.message : "API unavailable");
        });
    }, 0);
    const poll = window.setInterval(() => {
      refresh().catch(() => undefined);
    }, 10000);
    return () => {
      cancelled = true;
      window.clearTimeout(id);
      window.clearInterval(poll);
    };
  }, []);

  if (error && !auth) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-[var(--bg-0)] px-6">
        <div className="max-w-md rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-6 shadow-[var(--shadow)]">
          <h1 className="text-lg font-semibold">API unavailable</h1>
          <p className="mt-2 text-sm text-[var(--text-1)]">
            FleetDeck cannot reach the monitoring API. Start PostgreSQL and the API, then reload.
          </p>
          <p className="mt-3 font-[family-name:var(--font-mono-family)] text-xs text-[var(--text-2)]">
            {error}
          </p>
        </div>
      </div>
    );
  }

  if (!auth) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-[var(--bg-0)] text-[var(--text-2)]">
        Loading FleetDeck…
      </div>
    );
  }

  if (auth.bootstrap_required || !auth.authenticated) {
    return (
      <AuthGate
        status={auth}
        onDone={async () => {
          await refresh({ allowCookieSettle: true });
        }}
      />
    );
  }

  return (
    <div className="flex min-h-screen bg-[var(--bg-0)]">
      <ThemeSync />
      <CommandPalette />
      <Sidebar
        counts={{
          alerts: overview?.counts.critical_alerts ?? 0,
          unhealthy: overview?.counts.unhealthy_containers ?? 0,
          offline: overview?.counts.offline ?? 0,
        }}
      />
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 items-center justify-between border-b border-[var(--border)] bg-[var(--bg-1)] px-6">
          <div className="flex items-center gap-4 text-sm text-[var(--text-1)]">
            <span>
              Signed in as{" "}
              <span className="text-[var(--text-0)]">{auth.user?.display_name || auth.user?.email}</span>
            </span>
            <button
              type="button"
              className="rounded-md border border-[var(--border)] px-2 py-1 text-[11px] text-[var(--text-2)] hover:bg-[var(--bg-2)]"
              onClick={() => window.dispatchEvent(new Event("fleetdeck:open-search"))}
              title="Search (Ctrl+K)"
            >
              Ctrl+K
            </button>
          </div>
          <div className="flex items-center gap-2">
            <NotificationBell />
            <button
              className="rounded-md border border-[var(--border)] px-3 py-1.5 text-xs text-[var(--text-1)] hover:bg-[var(--bg-2)]"
              onClick={async () => {
                knownAuthed.current = false;
                await api.logout();
                await refresh();
              }}
            >
              Sign out
            </button>
          </div>
        </header>
        <main className="flex-1 overflow-auto p-6">{children}</main>
      </div>
    </div>
  );
}
