"use client";

import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/api";
import { useRealtime } from "@/lib/realtime";

type Summary = {
  servers: number;
  running: number;
  stopped: number;
  unhealthy: number;
  images: number;
  volumes: number;
  networks: number;
};

export default function DockerPage() {
  const [summary, setSummary] = useState<Summary | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback((opts?: { soft?: boolean }) => {
    api
      .dockerSummary()
      .then((s) => setSummary(s))
      .catch((e) => {
        if (!opts?.soft) setError(e instanceof Error ? e.message : "Failed to load");
      });
  }, []);

  useEffect(() => {
    const boot = window.setTimeout(() => load(), 0);
    const poll = window.setInterval(() => load({ soft: true }), 10000);
    return () => {
      window.clearTimeout(boot);
      window.clearInterval(poll);
    };
  }, [load]);

  useRealtime((type) => {
    if (type === "docker.updated" || type === "servers.updated") load({ soft: true });
  });

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <div>
        <div className="text-[11px] font-medium uppercase tracking-[0.18em] text-[var(--text-2)]">
          Docker infrastructure
        </div>
        <h1 className="mt-1 text-3xl font-semibold tracking-tight">Docker</h1>
        <p className="mt-1 text-sm text-[var(--text-1)]">
          Aggregated from enrolled agents. Empty means no Docker inventory has been reported yet — not mock data.
        </p>
      </div>

      {error && (
        <div className="rounded-md border border-[var(--crit)]/40 bg-[var(--crit)]/10 px-3 py-2 text-sm text-[var(--crit)]">
          {error}
        </div>
      )}

      {!summary ? (
        <div className="h-40 animate-pulse rounded-[var(--radius)] bg-[var(--bg-2)]" />
      ) : summary.servers === 0 && summary.running === 0 && summary.images === 0 ? (
        <div className="rounded-[var(--radius)] border border-dashed border-[var(--border-strong)] px-6 py-12 text-center text-sm text-[var(--text-1)]">
          No Docker hosts have reported inventory yet.
        </div>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <Tile label="Servers" value={summary.servers} />
          <Tile label="Running" value={summary.running} tone="ok" />
          <Tile label="Stopped" value={summary.stopped} />
          <Tile label="Unhealthy" value={summary.unhealthy} tone="warn" />
          <Tile label="Images" value={summary.images} />
          <Tile label="Volumes" value={summary.volumes} />
          <Tile label="Networks" value={summary.networks} />
        </div>
      )}
    </div>
  );
}

function Tile({ label, value, tone }: { label: string; value: number; tone?: "ok" | "warn" }) {
  const color = tone === "ok" ? "var(--ok)" : tone === "warn" ? "var(--warn)" : "var(--text-0)";
  return (
    <div className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-4">
      <div className="text-[11px] uppercase tracking-[0.12em] text-[var(--text-2)]">{label}</div>
      <div className="mt-2 font-[family-name:var(--font-mono-family)] text-2xl" style={{ color }}>
        {value}
      </div>
    </div>
  );
}
