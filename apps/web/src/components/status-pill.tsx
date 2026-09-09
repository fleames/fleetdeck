export function StatusPill({ state }: { state: string }) {
  const normalized = state.toLowerCase();
  const map: Record<string, { label: string; color: string }> = {
    healthy: { label: "Healthy", color: "var(--ok)" },
    online: { label: "Online", color: "var(--ok)" },
    warning: { label: "Warning", color: "var(--warn)" },
    critical: { label: "Critical", color: "var(--crit)" },
    offline: { label: "Offline", color: "var(--offline)" },
    maintenance: { label: "Maintenance", color: "var(--maint)" },
    pending: { label: "Pending", color: "var(--text-2)" },
    unknown: { label: "Unknown", color: "var(--text-2)" },
    outdated: { label: "Update available", color: "var(--warn)" },
  };
  const item = map[normalized] ?? { label: state, color: "var(--text-2)" };
  return (
    <span
      className="inline-flex items-center gap-1.5 rounded-full border border-[var(--border)] bg-[var(--bg-2)] px-2 py-0.5 text-[11px]"
      title={item.label}
    >
      <span
        className="inline-block h-1.5 w-1.5 rounded-full"
        style={{ background: item.color }}
        aria-hidden
      />
      <span>{item.label}</span>
    </span>
  );
}

/** True when enrolled agent reports a version different from the panel's expected release. */
export function agentVersionOutdated(
  agentVersion: string | null | undefined,
  currentVersion: string | null | undefined,
): boolean {
  const agent = (agentVersion ?? "").trim();
  const current = (currentVersion ?? "").trim();
  if (!agent || !current) return false;
  return agent !== current;
}

/** First agent release that handles panel `agent.update` + update path units. */
export const AGENT_PANEL_UPDATE_MIN = "0.4.2";

/** Manual CDN upgrade for hosts too old for panel Update agent (preserves credentials). */
export const AGENT_MANUAL_UPGRADE_CMD =
  "curl -fsSL https://cdn.tarkovbot.com/fleetdeck/upgrade.sh | sudo bash";

function parseAgentSemver(
  version: string | null | undefined,
): [number, number, number] | null {
  const raw = (version ?? "").trim();
  if (!raw) return null;
  const m = /^v?(\d+)\.(\d+)\.(\d+)/i.exec(raw);
  if (!m) return null;
  return [Number(m[1]), Number(m[2]), Number(m[3])];
}

function cmpSemver(a: [number, number, number], b: [number, number, number]): number {
  for (let i = 0; i < 3; i++) {
    if (a[i] !== b[i]) return a[i] < b[i] ? -1 : 1;
  }
  return 0;
}

/**
 * True when the agent version is known to support panel `agent.update`.
 * Unknown / unparsable versions return false (use manual upgrade.sh).
 */
export function agentSupportsPanelUpdate(
  agentVersion: string | null | undefined,
): boolean {
  const parsed = parseAgentSemver(agentVersion);
  const min = parseAgentSemver(AGENT_PANEL_UPDATE_MIN);
  if (!parsed || !min) return false;
  return cmpSemver(parsed, min) >= 0;
}
