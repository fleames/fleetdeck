"use client";

import { FormEvent, useState } from "react";
import { api, AuthStatus } from "@/lib/api";

const fieldClass =
  "w-full rounded-lg border border-[var(--border)] bg-[var(--bg-2)] px-3 py-2.5 text-sm text-[var(--text-0)]";

export function AuthGate({ status, onDone }: { status: AuthStatus; onDone: () => Promise<void> }) {
  const bootstrap = status.bootstrap_required;
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [displayName, setDisplayName] = useState("Admin");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      if (bootstrap) {
        await api.bootstrap(email, password, displayName);
      } else {
        await api.login(email, password);
      }
      await onDone();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Authentication failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-[var(--bg-0)] px-6">
      <form
        onSubmit={submit}
        className="w-full max-w-md rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-7 shadow-[var(--shadow)]"
      >
        <div className="text-[11px] font-medium uppercase tracking-[0.18em] text-[var(--text-2)]">
          FleetDeck
        </div>
        <h1 className="mt-2 text-2xl font-semibold tracking-tight">
          {bootstrap ? "Create admin account" : "Sign in"}
        </h1>
        <p className="mt-2 text-sm text-[var(--text-1)]">
          {bootstrap
            ? "First-run setup. Choose a strong password (12+ characters). No default credentials."
            : "Local authentication for your infrastructure command center."}
        </p>

        <div className="mt-6 space-y-3">
          {bootstrap && (
            <label className="block text-xs text-[var(--text-2)]">
              <span className="mb-1.5 block">Display name</span>
              <input
                className={fieldClass}
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
                required
              />
            </label>
          )}
          <label className="block text-xs text-[var(--text-2)]">
            <span className="mb-1.5 block">Email</span>
            <input
              className={fieldClass}
              type="email"
              autoComplete="username"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
            />
          </label>
          <label className="block text-xs text-[var(--text-2)]">
            <span className="mb-1.5 block">Password</span>
            <input
              className={fieldClass}
              type="password"
              autoComplete={bootstrap ? "new-password" : "current-password"}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              minLength={bootstrap ? 12 : 1}
              required
            />
          </label>
        </div>

        {error && (
          <p className="mt-4 rounded-md border border-[var(--crit)]/40 bg-[var(--crit)]/10 px-3 py-2 text-sm text-[var(--crit)]">
            {error}
          </p>
        )}

        <button
          type="submit"
          disabled={busy}
          className="mt-6 w-full rounded-md bg-[var(--accent)] px-4 py-2.5 text-sm font-medium text-white hover:brightness-110 disabled:opacity-60"
        >
          {busy ? "Working…" : bootstrap ? "Create account" : "Sign in"}
        </button>
      </form>
    </div>
  );
}
