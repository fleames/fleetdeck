"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { API_URL } from "@/lib/api";

type Server = { id: string; name: string; status: string };
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
  compose_project?: string;
};

export default function TopologyPage() {
  const [servers, setServers] = useState<Server[]>([]);
  const [compose, setCompose] = useState<Compose[]>([]);
  const [networks, setNetworks] = useState<Network[]>([]);
  const [containers, setContainers] = useState<Container[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const boot = window.setTimeout(() => {
      Promise.all([
        fetch(`${API_URL}/api/v1/servers`, { credentials: "include", cache: "no-store" }).then((r) =>
          r.ok ? r.json() : null,
        ),
        fetch(`${API_URL}/api/v1/compose`, { credentials: "include", cache: "no-store" }).then((r) =>
          r.ok ? r.json() : null,
        ),
        fetch(`${API_URL}/api/v1/networks`, { credentials: "include", cache: "no-store" }).then((r) =>
          r.ok ? r.json() : null,
        ),
        fetch(`${API_URL}/api/v1/containers`, { credentials: "include", cache: "no-store" }).then((r) =>
          r.ok ? r.json() : null,
        ),
      ])
        .then(([s, c, n, ct]) => {
          setServers(s?.data ?? []);
          setCompose(c?.data ?? []);
          setNetworks(n?.data ?? []);
          setContainers(ct?.data ?? []);
        })
        .catch((e) => setError(e instanceof Error ? e.message : "Failed to load"));
    }, 0);
    return () => window.clearTimeout(boot);
  }, []);

  const byServer = useMemo(() => {
    return servers.map((s) => ({
      server: s,
      projects: compose.filter((p) => p.server_id === s.id),
      nets: networks.filter((n) => n.server_id === s.id).slice(0, 8),
      running: containers.filter((c) => c.server_id === s.id && c.state === "running").length,
    }));
  }, [servers, compose, networks, containers]);

  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <div>
        <h1 className="text-3xl font-semibold tracking-tight">Topology</h1>
        <p className="mt-1 text-sm text-[var(--text-1)]">
          Fleet layout from live inventory: servers → Compose projects → networks. No synthetic nodes.
        </p>
      </div>
      {error && (
        <div className="rounded-md border border-[var(--crit)]/40 bg-[var(--crit)]/10 px-3 py-2 text-sm text-[var(--crit)]">
          {error}
        </div>
      )}
      {byServer.length === 0 ? (
        <div className="rounded-[var(--radius)] border border-dashed border-[var(--border-strong)] px-6 py-12 text-center text-sm text-[var(--text-1)]">
          No servers enrolled yet.
        </div>
      ) : (
        <div className="space-y-4">
          {byServer.map(({ server, projects, nets, running }) => (
            <section
              key={server.id}
              className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--bg-1)] p-5"
            >
              <div className="flex flex-wrap items-baseline justify-between gap-2">
                <Link href={`/servers/${server.id}`} className="text-lg font-semibold text-[var(--accent)]">
                  {server.name}
                </Link>
                <div className="text-xs text-[var(--text-2)]">
                  {server.status} · {running} running containers · {projects.length} compose · {nets.length}{" "}
                  networks
                </div>
              </div>
              <div className="mt-4 grid gap-3 md:grid-cols-2">
                <div>
                  <div className="mb-2 text-[11px] uppercase tracking-[0.14em] text-[var(--text-2)]">
                    Compose
                  </div>
                  {projects.length === 0 ? (
                    <div className="text-xs text-[var(--text-2)]">None reported</div>
                  ) : (
                    <ul className="space-y-1">
                      {projects.map((p) => (
                        <li
                          key={p.id}
                          className="rounded-md border border-[var(--border)] bg-[var(--bg-2)] px-3 py-2 text-sm"
                        >
                          {p.project_name}
                          <span className="ml-2 text-xs text-[var(--text-2)]">
                            {p.containers ?? 0} containers
                          </span>
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
                <div>
                  <div className="mb-2 text-[11px] uppercase tracking-[0.14em] text-[var(--text-2)]">
                    Networks
                  </div>
                  {nets.length === 0 ? (
                    <div className="text-xs text-[var(--text-2)]">None reported</div>
                  ) : (
                    <ul className="flex flex-wrap gap-2">
                      {nets.map((n) => (
                        <li
                          key={n.id}
                          className="rounded-md border border-[var(--border)] px-2 py-1 text-xs text-[var(--text-1)]"
                        >
                          {n.name}
                          {n.driver ? ` (${n.driver})` : ""}
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
              </div>
            </section>
          ))}
        </div>
      )}
    </div>
  );
}
