"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { API_URL } from "@/lib/api";
import { StatusPill } from "@/components/status-pill";
import { EmptyBlock, ErrorBanner, LoadingBlock } from "@/components/page-state";

type Server = { id: string; name: string; status: string; health_state?: string };
type Compose = {
  id: string;
  project_name: string;
  server_id: string;
  server_name?: string;
  status?: string;
  containers?: number;
};
type Network = { id: string; name: string; server_id: string; driver?: string };
type Container = {
  id: string;
  name: string;
  server_id: string;
  state: string;
  health?: string;
  compose_project?: string;
};

export default function TopologyPage() {
  const [servers, setServers] = useState<Server[]>([]);
  const [compose, setCompose] = useState<Compose[]>([]);
  const [networks, setNetworks] = useState<Network[]>([]);
  const [containers, setContainers] = useState<Container[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const boot = window.setTimeout(() => {
      Promise.all([
        fetch(`${API_URL}/api/v1/servers`, { credentials: "include", cache: "no-store" }).then((r) => {
          if (!r.ok) throw new Error(`Servers request failed (${r.status})`);
          return r.json();
        }),
        fetch(`${API_URL}/api/v1/compose`, { credentials: "include", cache: "no-store" }).then((r) => {
          if (!r.ok) throw new Error(`Compose request failed (${r.status})`);
          return r.json();
        }),
        fetch(`${API_URL}/api/v1/networks`, { credentials: "include", cache: "no-store" }).then((r) => {
          if (!r.ok) throw new Error(`Networks request failed (${r.status})`);
          return r.json();
        }),
        fetch(`${API_URL}/api/v1/containers`, { credentials: "include", cache: "no-store" }).then((r) => {
          if (!r.ok) throw new Error(`Containers request failed (${r.status})`);
          return r.json();
        }),
      ])
        .then(([s, c, n, ct]) => {
          setServers(s?.data ?? []);
          setCompose(c?.data ?? []);
          setNetworks(n?.data ?? []);
          setContainers(ct?.data ?? []);
          setError(null);
        })
        .catch((e) => setError(e instanceof Error ? e.message : "Failed to load topology"))
        .finally(() => setLoading(false));
    }, 0);
    return () => window.clearTimeout(boot);
  }, []);

  const byServer = useMemo(() => {
    return servers.map((s) => {
      const hostContainers = containers.filter((c) => c.server_id === s.id);
      const running = hostContainers.filter((c) => c.state === "running").length;
      const unhealthy = hostContainers.filter((c) => c.health === "unhealthy").length;
      return {
        server: s,
        projects: compose.filter((p) => p.server_id === s.id),
        nets: networks.filter((n) => n.server_id === s.id).slice(0, 12),
        running,
        total: hostContainers.length,
        unhealthy,
      };
    });
  }, [servers, compose, networks, containers]);

  const fleetTotals = useMemo(
    () => ({
      servers: servers.length,
      online: servers.filter((s) => s.status === "online").length,
      projects: compose.length,
      networks: networks.length,
      running: containers.filter((c) => c.state === "running").length,
    }),
    [servers, compose, networks, containers],
  );

  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <div>
        <div className="text-[11px] font-medium uppercase tracking-[0.18em] text-[var(--text-2)]">
          Inventory map
        </div>
        <h1 className="mt-1 text-3xl font-semibold tracking-tight">Topology</h1>
        <p className="mt-1 text-sm text-[var(--text-1)]">
          Fleet layout from live inventory: servers → Compose projects → networks. No synthetic nodes.
        </p>
      </div>

      {error && <ErrorBanner message={error} />}

      {loading && servers.length === 0 && !error ? (
        <LoadingBlock label="Loading topology" />
      ) : byServer.length === 0 ? (
        <EmptyBlock>
          No servers enrolled yet. Add a host on{" "}
          <Link href="/servers" className="text-[var(--accent)] underline-offset-2 hover:underline">
            Servers
          </Link>{" "}
          to populate this map.
        </EmptyBlock>
      ) : (
        <>
          <div
            className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5"
            aria-label="Fleet topology summary"
          >
            <SummaryTile label="Servers" value={`${fleetTotals.online}/${fleetTotals.servers}`} href="/servers" />
            <SummaryTile label="Compose" value={String(fleetTotals.projects)} href="/compose" />
            <SummaryTile label="Networks" value={String(fleetTotals.networks)} href="/networks" />
            <SummaryTile label="Running" value={String(fleetTotals.running)} href="/containers" />
            <SummaryTile label="Docker" value="Overview" href="/docker" />
          </div>

          <div className="space-y-4">
            {byServer.map(({ server, projects, nets, running, total, unhealthy }) => (
              <section
                key={server.id}
                aria-labelledby={`topo-server-${server.id}`}
                className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-5"
              >
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <div className="flex flex-wrap items-center gap-3">
                    <Link
                      id={`topo-server-${server.id}`}
                      href={`/servers/${server.id}`}
                      className="text-lg font-semibold text-[var(--accent)] underline-offset-2 hover:underline"
                    >
                      {server.name}
                    </Link>
                    <StatusPill state={server.health_state || server.status} />
                  </div>
                  <div className="text-xs text-[var(--text-2)]">
                    {running}/{total} containers running
                    {unhealthy > 0 ? ` · ${unhealthy} unhealthy` : ""}
                    {" · "}
                    {projects.length} compose · {nets.length} networks
                  </div>
                </div>

                <div className="mt-4 grid gap-4 md:grid-cols-2">
                  <div>
                    <div className="mb-2 flex items-center justify-between">
                      <h2 className="text-[11px] font-semibold uppercase tracking-[0.14em] text-[var(--text-2)]">
                        Compose
                      </h2>
                      <Link
                        href="/compose"
                        className="text-[11px] text-[var(--accent)] underline-offset-2 hover:underline"
                      >
                        View all
                      </Link>
                    </div>
                    {projects.length === 0 ? (
                      <p className="text-xs text-[var(--text-2)]">None reported</p>
                    ) : (
                      <ul className="space-y-1.5">
                        {projects.map((p) => (
                          <li
                            key={p.id}
                            className="flex items-center justify-between gap-2 rounded-md border border-[var(--border)] bg-[var(--bg-2)] px-3 py-2 text-sm"
                          >
                            <span className="truncate font-medium">{p.project_name}</span>
                            <span className="shrink-0 text-xs text-[var(--text-2)]">
                              {p.containers ?? 0} ctr
                            </span>
                          </li>
                        ))}
                      </ul>
                    )}
                  </div>
                  <div>
                    <div className="mb-2 flex items-center justify-between">
                      <h2 className="text-[11px] font-semibold uppercase tracking-[0.14em] text-[var(--text-2)]">
                        Networks
                      </h2>
                      <Link
                        href="/networks"
                        className="text-[11px] text-[var(--accent)] underline-offset-2 hover:underline"
                      >
                        View all
                      </Link>
                    </div>
                    {nets.length === 0 ? (
                      <p className="text-xs text-[var(--text-2)]">None reported</p>
                    ) : (
                      <ul className="flex flex-wrap gap-2">
                        {nets.map((n) => (
                          <li
                            key={n.id}
                            className="rounded-md border border-[var(--border)] bg-[var(--bg-2)] px-2.5 py-1 text-xs text-[var(--text-1)]"
                          >
                            <span className="text-[var(--text-0)]">{n.name}</span>
                            {n.driver ? (
                              <span className="ml-1 text-[var(--text-2)]">({n.driver})</span>
                            ) : null}
                          </li>
                        ))}
                      </ul>
                    )}
                  </div>
                </div>
              </section>
            ))}
          </div>
        </>
      )}
    </div>
  );
}

function SummaryTile({
  label,
  value,
  href,
}: {
  label: string;
  value: string;
  href: string;
}) {
  return (
    <Link
      href={href}
      className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] px-4 py-3 transition-colors hover:border-[var(--border-strong)] hover:bg-[var(--bg-2)]"
    >
      <div className="text-[11px] uppercase tracking-[0.12em] text-[var(--text-2)]">{label}</div>
      <div className="mt-1 font-[family-name:var(--font-mono-family)] text-lg text-[var(--text-0)]">
        {value}
      </div>
    </Link>
  );
}
