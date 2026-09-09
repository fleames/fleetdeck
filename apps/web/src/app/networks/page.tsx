"use client";

import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/api";
import { InventoryTable } from "@/components/inventory-table";
import { useRealtime } from "@/lib/realtime";

export default function NetworksPage() {
  const [rows, setRows] = useState<string[][]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const load = useCallback((opts?: { soft?: boolean }) => {
    api
      .networks()
      .then((d) => {
        setRows((d.data ?? []).map((n) => [n.name, n.driver, n.scope, n.server_name]));
        setError(null);
      })
      .catch((e) => {
        if (!opts?.soft) setError(e instanceof Error ? e.message : "Failed to load networks");
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
    <InventoryTable
      title="Networks"
      empty="No networks reported."
      headers={["Name", "Driver", "Scope", "Server"]}
      searchPlaceholder="Search name, driver, scope, server…"
      loading={loading}
      error={error}
      rows={rows}
    />
  );
}
