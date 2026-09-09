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
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const load = useCallback((opts?: { soft?: boolean }) => {
    api
      .images()
      .then((d) => {
        setRows(d.data ?? []);
        setError(null);
      })
      .catch((e) => {
        if (!opts?.soft) setError(e instanceof Error ? e.message : "Failed to load images");
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
      title="Images"
      empty="No images reported."
      headers={["Repository", "Tag", "Server", "Size", "Flags"]}
      searchPlaceholder="Search repository, tag, server…"
      loading={loading}
      error={error}
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
