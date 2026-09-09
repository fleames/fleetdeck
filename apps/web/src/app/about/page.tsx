"use client";

import { useEffect, useState } from "react";
import { API_URL } from "@/lib/api";

type SelfMetrics = {
  api?: { status?: string; uptime_seconds?: number; started_at?: string; checked_at?: string };
  fleet?: {
    servers?: number;
    online_servers?: number;
    online_agents?: number;
    active_alerts?: number;
  };
  ingestion?: { server_metric_points_last_hour?: number };
};

export default function AboutPage() {
  const [self, setSelf] = useState<SelfMetrics | null>(null);

  useEffect(() => {
    const boot = window.setTimeout(() => {
      fetch(`${API_URL}/api/v1/overview/self`, { credentials: "include", cache: "no-store" })
        .then((r) => (r.ok ? r.json() : null))
        .then((d) => setSelf(d))
        .catch(() => undefined);
    }, 0);
    return () => window.clearTimeout(boot);
  }, []);

  return (
    <div className="mx-auto max-w-3xl space-y-4">
      <h1 className="text-3xl font-semibold tracking-tight">About FleetDeck</h1>
      <p className="text-sm text-[var(--text-1)]">
        Local-first server monitoring and Docker management. Agents dial out; the Docker socket stays on
        the host; the dashboard never receives secrets or socket access.
      </p>
      <dl className="grid gap-3 rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-5 text-sm">
        <div className="flex justify-between gap-4">
          <dt className="text-[var(--text-2)]">Product</dt>
          <dd>FleetDeck</dd>
        </div>
        <div className="flex justify-between gap-4">
          <dt className="text-[var(--text-2)]">Deployment model</dt>
          <dd>Self-hosted / local-first</dd>
        </div>
        <div className="flex justify-between gap-4">
          <dt className="text-[var(--text-2)]">Agent protocol</dt>
          <dd>Enrollment, heartbeat, metrics, inventory, commands</dd>
        </div>
        <div className="flex justify-between gap-4">
          <dt className="text-[var(--text-2)]">Fake production data</dt>
          <dd>Disabled</dd>
        </div>
        {self?.api && (
          <>
            <div className="flex justify-between gap-4">
              <dt className="text-[var(--text-2)]">API status</dt>
              <dd>{self.api.status}</dd>
            </div>
            <div className="flex justify-between gap-4">
              <dt className="text-[var(--text-2)]">API uptime</dt>
              <dd>{self.api.uptime_seconds ?? 0}s</dd>
            </div>
          </>
        )}
        {self?.fleet && (
          <div className="flex justify-between gap-4">
            <dt className="text-[var(--text-2)]">Fleet snapshot</dt>
            <dd>
              {self.fleet.online_servers}/{self.fleet.servers} servers online ·{" "}
              {self.fleet.online_agents} agents · {self.fleet.active_alerts} alerts
            </dd>
          </div>
        )}
      </dl>
    </div>
  );
}
