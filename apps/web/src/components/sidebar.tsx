"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

const nav = [
  {
    label: "Overview",
    items: [
      { href: "/", label: "Dashboard" },
      { href: "/servers", label: "Servers" },
      { href: "/compare", label: "Compare" },
      { href: "/topology", label: "Topology" },
      { href: "/docker", label: "Docker" },
      { href: "/containers", label: "Containers" },
      { href: "/compose", label: "Compose" },
      { href: "/images", label: "Images" },
      { href: "/volumes", label: "Volumes" },
      { href: "/networks", label: "Networks" },
    ],
  },
  {
    label: "Observability",
    items: [
      { href: "/metrics", label: "Metrics" },
      { href: "/alerts", label: "Alerts" },
      { href: "/events", label: "Events" },
      { href: "/logs", label: "Logs" },
    ],
  },
  {
    label: "System",
    items: [
      { href: "/settings", label: "Settings" },
      { href: "/agents", label: "Agents" },
      { href: "/users", label: "Users" },
      { href: "/audit", label: "Audit Log" },
      { href: "/about", label: "About" },
    ],
  },
];

export function Sidebar({
  counts,
}: {
  counts?: { alerts: number; unhealthy: number; offline: number };
}) {
  const pathname = usePathname();

  return (
    <aside
      className="flex w-60 shrink-0 flex-col border-r border-[var(--border)] bg-[var(--bg-1)]"
      aria-label="Primary"
    >
      <div className="border-b border-[var(--border)] px-5 py-5">
        <div className="text-[11px] font-medium uppercase tracking-[0.18em] text-[var(--text-2)]">
          FleetDeck
        </div>
        <div className="mt-1 font-[family-name:var(--font-mono-family)] text-sm text-[var(--text-1)]">
          Infrastructure
        </div>
      </div>

      <div
        className="space-y-2 border-b border-[var(--border)] px-4 py-3 text-xs"
        aria-label="Fleet status counts"
      >
        <CountRow label="Alerts" value={counts?.alerts ?? 0} tone={counts?.alerts ? "crit" : "muted"} />
        <CountRow
          label="Unhealthy"
          value={counts?.unhealthy ?? 0}
          tone={counts?.unhealthy ? "warn" : "muted"}
        />
        <CountRow
          label="Offline"
          value={counts?.offline ?? 0}
          tone={counts?.offline ? "crit" : "muted"}
        />
      </div>

      <nav className="flex-1 overflow-y-auto px-3 py-4" aria-label="Main">
        {nav.map((section) => (
          <div key={section.label} className="mb-5">
            <div
              id={`nav-${section.label.toLowerCase()}`}
              className="mb-2 px-2 text-[10px] font-semibold uppercase tracking-[0.16em] text-[var(--text-2)]"
            >
              {section.label}
            </div>
            <ul className="space-y-0.5" aria-labelledby={`nav-${section.label.toLowerCase()}`}>
              {section.items.map((item) => {
                const active = pathname === item.href || (item.href !== "/" && pathname.startsWith(`${item.href}/`));
                return (
                  <li key={item.href}>
                    <Link
                      href={item.href}
                      aria-current={active ? "page" : undefined}
                      className={`block rounded-md px-2.5 py-1.5 text-sm transition-colors ${
                        active
                          ? "bg-[var(--bg-3)] text-[var(--text-0)]"
                          : "text-[var(--text-1)] hover:bg-[var(--bg-2)] hover:text-[var(--text-0)]"
                      }`}
                    >
                      {item.label}
                    </Link>
                  </li>
                );
              })}
            </ul>
          </div>
        ))}
      </nav>
    </aside>
  );
}

function CountRow({
  label,
  value,
  tone,
}: {
  label: string;
  value: number;
  tone: "crit" | "warn" | "muted";
}) {
  const color =
    tone === "crit" ? "var(--crit)" : tone === "warn" ? "var(--warn)" : "var(--text-2)";
  return (
    <div className="flex items-center justify-between rounded-md bg-[var(--bg-2)] px-2.5 py-1.5">
      <span className="text-[var(--text-2)]">{label}</span>
      <span className="font-[family-name:var(--font-mono-family)]" style={{ color }}>
        {value}
      </span>
    </div>
  );
}
