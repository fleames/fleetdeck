/** First agent release that handles panel `agent.update` + update path units. */
export const AGENT_PANEL_UPDATE_MIN = "0.4.2";

/** Manual CDN upgrade for hosts too old for panel Update agent (preserves credentials). */
export function manualUpgradeCommand(
  cdnBase = "https://cdn.example.com/fleetdeck",
): string {
  const base = cdnBase.replace(/\/+$/, "");
  return `curl -fsSL ${base}/upgrade.sh | sudo bash`;
}

/** Default one-liner; prefer `manualUpgradeCommand(configuredCdnBase)` when CDN is known. */
export const AGENT_MANUAL_UPGRADE_CMD = manualUpgradeCommand();

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
 * True when the enrolled agent is older than the panel expected release
 * (major.minor.patch; suffixes like `-dev` ignored). Equal or newer → false.
 */
export function agentVersionOutdated(
  agentVersion: string | null | undefined,
  currentVersion: string | null | undefined,
): boolean {
  const agent = (agentVersion ?? "").trim();
  const current = (currentVersion ?? "").trim();
  if (!agent || !current) return false;
  if (agent === current) return false;

  const agentSem = parseAgentSemver(agent);
  const currentSem = parseAgentSemver(current);
  if (agentSem && currentSem) {
    return cmpSemver(agentSem, currentSem) < 0;
  }
  // Unparsable versions: fall back to exact match (treat unknown inequality as outdated).
  return true;
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
