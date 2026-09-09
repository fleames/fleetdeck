"use client";

import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/api";
import { formatBytes } from "@/lib/format";
import { InventoryTable } from "@/components/inventory-table";
import { useRealtime } from "@/lib/realtime";

type ImageRow = {
  id: string;
  server_name: string;
  repository: string;
  tag: string;
  size_bytes: number;
  dangling: boolean;
};

export default function ImagesPage() {
  const [rows, setRows] = useState<ImageRow[]>([]);

  const load = useCallback(() => {
    api
      .images()
      .then((d) => setRows(d.data ?? []))
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
      title="Images"
      empty="No images reported."
      headers={["Repository", "Tag", "Server", "Size", "Flags"]}
      rows={rows.map((r) => [
        r.repository,
        r.tag,
        r.server_name,
        formatBytes(r.size_bytes),
        r.dangling ? "dangling" : "",
      ])}
    />
  );
}
