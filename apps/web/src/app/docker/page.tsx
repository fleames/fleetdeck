"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/api";
import { useRealtime } from "@/lib/realtime";
import { EmptyBlock, ErrorBanner, LoadingBlock } from "@/components/page-state";

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
  const [loading, setLoading] = useState(true);

  const load = useCallback((opts?: { soft?: boolean }) => {
    api
      .dockerSummary()
      .then((s) => {
        setSummary(s);
        setError(null);
      })
      .catch((e) => {
        if (!opts?.soft) setError(e instanceof Error ? e.message : "Failed to load");
      })
      .finally(() => {
        if (!opts?.soft) setLoading(false);
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

      {error && <ErrorBanner message={error} />}

      {loading && !summary ? (
        <LoadingBlock label="Loading Docker summary" />
      ) : !summary ||
        (summary.servers === 0 && summary.running === 0 && summary.images === 0) ? (
        <EmptyBlock>No Docker hosts have reported inventory yet.</EmptyBlock>
      ) : (
        <>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <Tile label="Servers" value={summary.servers} href="/servers" />
            <Tile label="Running" value={summary.running} tone="ok" href="/containers?filter=running" />
            <Tile label="Stopped" value={summary.stopped} href="/containers?filter=stopped" />
            <Tile label="Unhealthy" value={summary.unhealthy} tone="warn" href="/containers?filter=unhealthy" />
            <Tile label="Images" value={summary.images} href="/images" />
            <Tile label="Volumes" value={summary.volumes} href="/volumes" />
            <Tile label="Networks" value={summary.networks} href="/networks" />
          </div>
          <p className="text-xs text-[var(--text-2)]">
            Open Containers / Images / Volumes / Networks for searchable tables.
          </p>
        </>
      )}
    </div>
  );
}

function Tile({
  label,
  value,
  tone,
  href,
}: {
  label: string;
  value: number;
  tone?: "ok" | "warn";
  href: string;
}) {
  const color = tone === "ok" ? "var(--ok)" : tone === "warn" ? "var(--warn)" : "var(--text-0)";
  return (
    <Link
      href={href}
      className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-4 transition-colors hover:border-[var(--border-strong)]"
    >
      <div className="text-[11px] uppercase tracking-[0.12em] text-[var(--text-2)]">{label}</div>
      <div className="mt-2 font-[family-name:var(--font-mono-family)] text-2xl" style={{ color }}>
        {value}
      </div>
    </Link>
  );
}
