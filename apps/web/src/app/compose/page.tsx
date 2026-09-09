"use client";

import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/api";
import { InventoryTable } from "@/components/inventory-table";
import { useRealtime } from "@/lib/realtime";

export default function ComposePage() {
  const [rows, setRows] = useState<string[][]>([]);

  const load = useCallback(() => {
    api
      .compose()
      .then((d) =>
        setRows(
          (d.data ?? []).map((p) => [
            p.project_name,
            p.status,
            p.server_name,
            String(p.containers),
          ]),
        ),
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
      title="Compose"
      empty="No Compose projects detected yet."
      headers={["Project", "Status", "Server", "Containers"]}
      rows={rows}
    />
  );
}
