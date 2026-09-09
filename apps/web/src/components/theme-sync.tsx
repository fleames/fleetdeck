"use client";

import { useEffect } from "react";
import { API_URL } from "@/lib/api";

/** Applies persisted theme from settings when authenticated shell mounts. */
export function ThemeSync() {
  useEffect(() => {
    let cancelled = false;
    fetch(`${API_URL}/api/v1/settings`, { credentials: "include", cache: "no-store" })
      .then((r) => (r.ok ? r.json() : null))
      .then((data) => {
        if (cancelled || !data?.general?.theme) return;
        const theme = data.general.theme === "light" ? "light" : "dark";
        document.documentElement.setAttribute("data-theme", theme);
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, []);
  return null;
}
