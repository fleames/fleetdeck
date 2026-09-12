export {
  AGENT_MANUAL_UPGRADE_CMD,
  AGENT_PANEL_UPDATE_MIN,
  agentSupportsPanelUpdate,
  agentVersionOutdated,
} from "@/lib/agent-version";

export function StatusPill({ state }: { state: string }) {
  const normalized = state.toLowerCase();
  const map: Record<string, { label: string; color: string }> = {
    healthy: { label: "Healthy", color: "var(--ok)" },
    online: { label: "Online", color: "var(--ok)" },
    running: { label: "Running", color: "var(--ok)" },
    warning: { label: "Warning", color: "var(--warn)" },
    critical: { label: "Critical", color: "var(--crit)" },
    offline: { label: "Offline", color: "var(--offline)" },
    exited: { label: "Exited", color: "var(--warn)" },
    dead: { label: "Dead", color: "var(--crit)" },
    created: { label: "Created", color: "var(--text-2)" },
    paused: { label: "Paused", color: "var(--warn)" },
    restarting: { label: "Restarting", color: "var(--warn)" },
    detected: { label: "Detected", color: "var(--text-2)" },
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
      role="status"
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
