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

/** Operator-facing freshness for LIVE / RECENT / STALE / OFFLINE. */
export function freshnessLabel(
  lastUpdated?: string | null,
  status?: string,
  nowMs: number = Date.now(),
): {
  label: string;
  tone: "ok" | "warn" | "crit" | "muted";
  code: "LIVE" | "RECENT" | "STALE" | "OFFLINE" | "NONE";
} {
  if (status === "offline") return { label: "OFFLINE", tone: "crit", code: "OFFLINE" };
  if (!lastUpdated) return { label: "No metrics yet", tone: "muted", code: "NONE" };
  const ageMs = nowMs - new Date(lastUpdated).getTime();
  const ageSec = Math.max(1, Math.round(ageMs / 1000));
  if (ageMs < 30_000) return { label: `LIVE · ${ageSec}s`, tone: "ok", code: "LIVE" };
  if (ageMs < 120_000) return { label: `RECENT · ${ageSec}s`, tone: "warn", code: "RECENT" };
  return { label: `STALE · ${ageSec}s`, tone: "crit", code: "STALE" };
}
