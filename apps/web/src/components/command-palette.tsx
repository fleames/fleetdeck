"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { API_URL } from "@/lib/api";

type Hit = { id: string; title: string; subtitle: string; href: string };
type SearchResult = {
  servers: Hit[];
  containers: Hit[];
  images: Hit[];
  compose: Hit[];
  volumes: Hit[];
  networks: Hit[];
  alerts: Hit[];
  events: Hit[];
};

export function CommandPalette() {
  const [open, setOpen] = useState(false);
  const [q, setQ] = useState("");
  const [data, setData] = useState<SearchResult | null>(null);
  const router = useRouter();

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setOpen((v) => !v);
      }
      if (e.key === "Escape") setOpen(false);
    }
    function onOpen() {
      setOpen(true);
    }
    window.addEventListener("keydown", onKey);
    window.addEventListener("fleetdeck:open-search", onOpen);
    return () => {
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("fleetdeck:open-search", onOpen);
    };
  }, []);

  useEffect(() => {
    if (!open) return;
    const t = window.setTimeout(() => {
      fetch(`${API_URL}/api/v1/search?q=${encodeURIComponent(q)}`, {
        credentials: "include",
        cache: "no-store",
      })
        .then((r) => r.json())
        .then((d) => setData(d))
        .catch(() => setData(null));
    }, 120);
    return () => window.clearTimeout(t);
  }, [q, open]);

  const sections = useMemo(() => {
    if (!data) return [];
    return [
      { label: "Servers", items: data.servers },
      { label: "Containers", items: data.containers },
      { label: "Images", items: data.images },
      { label: "Compose", items: data.compose },
      { label: "Volumes", items: data.volumes },
      { label: "Networks", items: data.networks },
      { label: "Alerts", items: data.alerts },
      { label: "Events", items: data.events },
    ].filter((s) => s.items?.length);
  }, [data]);

  const go = useCallback(
    (href: string) => {
      setOpen(false);
      setQ("");
      router.push(href);
    },
    [router],
  );

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center bg-black/55 px-4 pt-[12vh]"
      onClick={() => setOpen(false)}
      role="dialog"
      aria-modal="true"
      aria-label="Command palette"
    >
      <div
        className="w-full max-w-xl overflow-hidden rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] shadow-[var(--shadow)]"
        onClick={(e) => e.stopPropagation()}
      >
        <input
          autoFocus
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="Search servers, containers, images…"
          className="w-full border-b border-[var(--border)] bg-transparent px-4 py-3 text-sm outline-none"
        />
        <div className="max-h-[50vh] overflow-y-auto p-2">
          {sections.length === 0 ? (
            <div className="px-3 py-6 text-center text-sm text-[var(--text-2)]">
              {q ? "No matches." : "Type to search your fleet."}
            </div>
          ) : (
            sections.map((section) => (
              <div key={section.label} className="mb-2">
                <div className="px-2 py-1 text-[10px] font-semibold uppercase tracking-[0.14em] text-[var(--text-2)]">
                  {section.label}
                </div>
                <ul>
                  {section.items.map((item) => (
                    <li key={item.id}>
                      <button
                        type="button"
                        className="flex w-full flex-col rounded-md px-2 py-2 text-left hover:bg-[var(--bg-2)]"
                        onClick={() => go(item.href)}
                      >
                        <span className="text-sm text-[var(--text-0)]">{item.title}</span>
                        <span className="text-xs text-[var(--text-2)]">{item.subtitle}</span>
                      </button>
                    </li>
                  ))}
                </ul>
              </div>
            ))
          )}
        </div>
        <div className="border-t border-[var(--border)] px-3 py-2 text-[11px] text-[var(--text-2)]">
          <kbd className="rounded border border-[var(--border)] px-1">Ctrl</kbd>+
          <kbd className="rounded border border-[var(--border)] px-1">K</kbd> to toggle · Esc to close ·{" "}
          <Link href="/servers" className="text-[var(--accent)]" onClick={() => setOpen(false)}>
            Servers
          </Link>
        </div>
      </div>
    </div>
  );
}
