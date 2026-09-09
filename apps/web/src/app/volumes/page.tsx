"use client";

import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/api";
import { InventoryTable } from "@/components/inventory-table";
import { useRealtime } from "@/lib/realtime";

export default function VolumesPage() {
  const [rows, setRows] = useState<string[][]>([]);

  const load = useCallback(() => {
    api
      .volumes()
      .then((d) =>
        setRows(
          (d.data ?? []).map((v) => [v.name, v.driver, v.server_name, v.unused ? "unused" : ""]),
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
    <InventoryTable title="Volumes" empty="No volumes reported." headers={["Name", "Driver", "Server", "Flags"]} rows={rows} />
  );
}
