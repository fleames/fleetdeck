"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { API_URL, apiFetch } from "@/lib/api";

type Note = {
  id: string;
  kind: string;
  title: string;
  body: string;
  created_at: string;
  href: string;
};

export function NotificationBell() {
  const [open, setOpen] = useState(false);
  const [notes, setNotes] = useState<Note[]>([]);
  const [busy, setBusy] = useState(false);

  async function load() {
    try {
      const res = await fetch(`${API_URL}/api/v1/notifications`, {
        credentials: "include",
        cache: "no-store",
      });
      if (!res.ok) return;
      const data = await res.json();
      setNotes(data.data ?? []);
    } catch {
      // ignore
    }
  }

  useEffect(() => {
    const boot = window.setTimeout(() => void load(), 0);
    const poll = window.setInterval(() => void load(), 15000);
    return () => {
      window.clearTimeout(boot);
      window.clearInterval(poll);
    };
  }, []);

  async function dismissOne(id: string) {
    if (busy) return;
    setBusy(true);
    setNotes((prev) => prev.filter((n) => n.id !== id));
    try {
      await apiFetch(`/api/v1/notifications/${encodeURIComponent(id)}/dismiss`, { method: "POST" });
    } catch {
      await load();
    } finally {
      setBusy(false);
    }
  }

  async function clearAll() {
    if (busy || notes.length === 0) return;
    setBusy(true);
    const prev = notes;
    setNotes([]);
    try {
      await apiFetch("/api/v1/notifications/clear", { method: "POST" });
    } catch {
      setNotes(prev);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="relative">
      <button
        type="button"
        className="relative rounded-md border border-[var(--border)] px-2 py-1 text-[11px] text-[var(--text-2)] hover:bg-[var(--bg-2)]"
        onClick={() => setOpen((v) => !v)}
        aria-label="Notifications"
      >
        Alerts
        {notes.length > 0 && (
          <span className="ml-1 rounded-full bg-[var(--crit)] px-1.5 text-[10px] text-white">
            {Math.min(99, notes.length)}
          </span>
        )}
      </button>
      {open && (
        <div className="absolute right-0 z-40 mt-2 w-80 overflow-hidden rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] shadow-[var(--shadow)]">
          <div className="flex items-center justify-between border-b border-[var(--border)] px-3 py-2">
            <div className="text-xs font-medium text-[var(--text-2)]">Notification center</div>
            {notes.length > 0 && (
              <button
                type="button"
                className="text-[10px] text-[var(--text-2)] hover:text-[var(--text-0)] disabled:opacity-50"
                onClick={() => void clearAll()}
                disabled={busy}
              >
                Clear all
              </button>
            )}
          </div>
          <div className="max-h-80 overflow-y-auto">
            {notes.length === 0 ? (
              <div className="px-3 py-6 text-center text-xs text-[var(--text-2)]">No active notifications.</div>
            ) : (
              notes.slice(0, 15).map((n) => (
                <div
                  key={n.id}
                  className="flex items-start gap-2 border-b border-[var(--border)] px-3 py-2 hover:bg-[var(--bg-2)]"
                >
                  <Link
                    href={n.href}
                    className="min-w-0 flex-1"
                    onClick={() => setOpen(false)}
                  >
                    <div className="text-xs font-medium text-[var(--text-0)]">{n.title}</div>
                    <div className="mt-0.5 text-xs text-[var(--text-1)]">{n.body}</div>
                    <div className="mt-1 text-[10px] text-[var(--text-2)]">
                      {new Date(n.created_at).toLocaleString()}
                    </div>
                  </Link>
                  <button
                    type="button"
                    className="shrink-0 rounded px-1.5 py-0.5 text-[10px] text-[var(--text-2)] hover:bg-[var(--bg-0)] hover:text-[var(--text-0)] disabled:opacity-50"
                    aria-label={`Dismiss ${n.title}`}
                    title="Dismiss"
                    onClick={() => void dismissOne(n.id)}
                    disabled={busy}
                  >
                    ×
                  </button>
                </div>
              ))
            )}
          </div>
          <div className="px-3 py-2 text-[10px] text-[var(--text-2)]">
            External alerts: configure webhook URL in Settings (Discord/Slack compatible). Local-only by default.
          </div>
        </div>
      )}
    </div>
  );
}
