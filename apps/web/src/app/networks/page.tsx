"use client";

import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/api";
import { InventoryTable } from "@/components/inventory-table";
import { useRealtime } from "@/lib/realtime";

export default function NetworksPage() {
  const [rows, setRows] = useState<string[][]>([]);

  const load = useCallback(() => {
    api
      .networks()
      .then((d) =>
        setRows((d.data ?? []).map((n) => [n.name, n.driver, n.scope, n.server_name])),
      )
      .catch(() => undefined);
  }, []);

  useEffect(() => {
    const boot = window.setTimeout(load, 0);
    const poll = window.setInterval(load, 10000);
    return () => {
      window.clearTimeout(boot);
      window.clearInterval(poll);
    };
  }, [load]);

  useRealtime((type) => {
    if (type === "docker.updated" || type === "servers.updated") load();
  });

  return (
    <InventoryTable
      title="Networks"
      empty="No networks reported."
      headers={["Name", "Driver", "Scope", "Server"]}
      rows={rows}
    />
  );
}
