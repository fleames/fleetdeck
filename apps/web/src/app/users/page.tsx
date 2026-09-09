"use client";

import { FormEvent, useEffect, useState } from "react";
import { API_URL, apiFetch } from "@/lib/api";

type UserRow = {
  id: string;
  email: string;
  display_name: string;
  role: string;
  created_at: string;
};

const fieldClass =
  "w-full rounded-lg border border-[var(--border)] bg-[var(--bg-2)] px-3 py-2 text-sm text-[var(--text-0)]";

export default function UsersPage() {
  const [users, setUsers] = useState<UserRow[]>([]);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [role, setRole] = useState("viewer");
  const [error, setError] = useState<string | null>(null);
  const [msg, setMsg] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function load() {
    const res = await fetch(`${API_URL}/api/v1/users`, { credentials: "include", cache: "no-store" });
    if (!res.ok) throw new Error("Could not load users (admin only)");
    const data = await res.json();
    setUsers(data.data ?? []);
  }

  useEffect(() => {
    const boot = window.setTimeout(() => {
      load().catch((e) => setError(e instanceof Error ? e.message : "Failed"));
    }, 0);
    return () => window.clearTimeout(boot);
  }, []);

  async function onCreate(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    setMsg(null);
    try {
      const res = await apiFetch(`/api/v1/users`, {
        method: "POST",
        body: JSON.stringify({
          email,
          password,
          display_name: displayName || email,
          role,
        }),
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data?.error?.message ?? "Create failed");
      setMsg(`Created ${data.user.email} as ${data.user.role}`);
      setEmail("");
      setPassword("");
      setDisplayName("");
      setRole("viewer");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Create failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <div>
        <h1 className="text-3xl font-semibold tracking-tight">Users</h1>
        <p className="mt-1 text-sm text-[var(--text-1)]">
          Local accounts. Roles are enforced by the API (admin / operator / viewer).
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
        onSubmit={onCreate}
        className="space-y-3 rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-5"
      >
        <div className="grid gap-3 sm:grid-cols-2">
          <label className="block text-xs text-[var(--text-2)]">
            <span className="mb-1.5 block">Email</span>
            <input className={fieldClass} type="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
          </label>
          <label className="block text-xs text-[var(--text-2)]">
            <span className="mb-1.5 block">Display name</span>
            <input className={fieldClass} value={displayName} onChange={(e) => setDisplayName(e.target.value)} />
          </label>
          <label className="block text-xs text-[var(--text-2)]">
            <span className="mb-1.5 block">Password (≥12)</span>
            <input
              className={fieldClass}
              type="password"
              required
              minLength={12}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </label>
          <label className="block text-xs text-[var(--text-2)]">
            <span className="mb-1.5 block">Role</span>
            <select className={fieldClass} value={role} onChange={(e) => setRole(e.target.value)}>
              <option value="viewer">Viewer</option>
              <option value="operator">Operator</option>
              <option value="admin">Admin</option>
            </select>
          </label>
        </div>
        <button
          type="submit"
          disabled={busy}
          className="rounded-md bg-[var(--accent)] px-4 py-2 text-sm text-white disabled:opacity-60"
        >
          {busy ? "Creating…" : "Create user"}
        </button>
      </form>

      <div className="overflow-hidden rounded-[var(--radius)] border border-[var(--border)]">
        <table className="w-full text-left text-sm">
          <thead className="bg-[var(--bg-2)] text-xs text-[var(--text-2)]">
            <tr>
              <th className="px-4 py-2 font-medium">User</th>
              <th className="px-4 py-2 font-medium">Role</th>
              <th className="px-4 py-2 font-medium">Created</th>
            </tr>
          </thead>
          <tbody>
            {users.map((u) => (
              <tr key={u.id} className="border-t border-[var(--border)] bg-[var(--bg-1)]">
                <td className="px-4 py-2">
                  <div className="font-medium">{u.display_name}</div>
                  <div className="text-xs text-[var(--text-2)]">{u.email}</div>
                </td>
                <td className="px-4 py-2 capitalize">{u.role}</td>
                <td className="px-4 py-2 text-[var(--text-2)]">{new Date(u.created_at).toLocaleString()}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
