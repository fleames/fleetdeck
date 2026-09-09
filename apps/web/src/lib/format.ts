export function pct(used?: number | null, total?: number | null): number | null {
  if (used == null || total == null || total <= 0) return null;
  return Math.max(0, Math.min(100, (used / total) * 100));
}

export function formatBytes(n?: number | null): string {
  if (n == null) return "—";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let v = n;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i += 1;
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

export function formatBps(n?: number | null): string {
  if (n == null) return "—";
  const bits = n * 8;
  const units = ["bps", "Kbps", "Mbps", "Gbps"];
  let v = bits;
  let i = 0;
  while (v >= 1000 && i < units.length - 1) {
    v /= 1000;
    i += 1;
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

export function formatUptime(seconds?: number | null): string {
  if (seconds == null) return "—";
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  if (d > 0) return `${d}d ${h}h`;
  const m = Math.floor((seconds % 3600) / 60);
  return `${h}h ${m}m`;
}

export function freshnessLabel(
  lastUpdated?: string | null,
  status?: string,
): {
  label: string;
  tone: "ok" | "warn" | "crit" | "muted";
} {
  if (status === "offline") return { label: "Offline", tone: "crit" };
  if (!lastUpdated) return { label: "No metrics yet", tone: "muted" };
  const ageMs = Date.now() - new Date(lastUpdated).getTime();
  if (ageMs < 30_000)
    return { label: `Updated ${Math.max(1, Math.round(ageMs / 1000))}s ago`, tone: "ok" };
  if (ageMs < 120_000) return { label: `Delayed · ${Math.round(ageMs / 1000)}s`, tone: "warn" };
  return { label: `Stale · ${Math.round(ageMs / 1000)}s ago`, tone: "crit" };
}
