/**
 * Empty / unset NEXT_PUBLIC_API_URL → same-origin relative URLs.
 * Session cookies are then set on the web origin via Next rewrites (see next.config.ts).
 * Direct http://localhost:8080 is still supported for local API-only debugging.
 */
const rawApi = process.env.NEXT_PUBLIC_API_URL;
export const API_URL = (rawApi === undefined || rawApi === "" ? "" : rawApi).replace(/\/$/, "");

/** WebSocket cannot rely on Next rewrites; dial the API host directly. */
export function realtimeWsURL(): string {
  const explicit = process.env.NEXT_PUBLIC_WS_URL || (API_URL || undefined);
  if (explicit) {
    return explicit.replace(/^http/, "ws") + "/api/v1/realtime";
  }
  if (typeof window === "undefined") {
    return "ws://127.0.0.1:8080/api/v1/realtime";
  }
  const proto = window.location.protocol === "https:" ? "wss" : "ws";
  const host = window.location.hostname;
  return `${proto}://${host}:8080/api/v1/realtime`;
}

const CSRF_KEY = "fleetdeck_csrf";

export type ApiError = {
  error: {
    code: string;
    message: string;
    details?: Record<string, unknown>;
    diagnostics?: Record<string, unknown>;
  };
};

export function getCsrfToken(): string | null {
  if (typeof window === "undefined") return null;
  return sessionStorage.getItem(CSRF_KEY);
}

export function setCsrfToken(token: string | null | undefined) {
  if (typeof window === "undefined") return;
  if (!token) {
    sessionStorage.removeItem(CSRF_KEY);
    return;
  }
  sessionStorage.setItem(CSRF_KEY, token);
}

export async function ensureCsrfToken(): Promise<string | null> {
  const existing = getCsrfToken();
  if (existing) return existing;
  try {
    const res = await fetch(`${API_URL}/api/v1/auth/csrf`, {
      credentials: "include",
      cache: "no-store",
    });
    if (!res.ok) return null;
    const data = (await res.json()) as { csrf_token?: string };
    if (data.csrf_token) setCsrfToken(data.csrf_token);
    return data.csrf_token ?? null;
  } catch {
    return null;
  }
}

/** Shared fetch with credentials + CSRF on mutating methods. */
export async function apiFetch(path: string, init?: RequestInit): Promise<Response> {
  const method = (init?.method ?? "GET").toUpperCase();
  const headers = new Headers(init?.headers);
  if (!headers.has("Content-Type") && init?.body) {
    headers.set("Content-Type", "application/json");
  }
  if (!["GET", "HEAD", "OPTIONS"].includes(method)) {
    const csrf = (await ensureCsrfToken()) || getCsrfToken();
    if (csrf) headers.set("X-CSRF-Token", csrf);
  }
  return fetch(`${API_URL}${path}`, {
    ...init,
    credentials: "include",
    headers,
    cache: "no-store",
  });
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await apiFetch(path, init);
  const data = await res.json().catch(() => ({}));
  if (data && typeof data === "object" && "csrf_token" in data && typeof data.csrf_token === "string") {
    setCsrfToken(data.csrf_token);
  }
  if (!res.ok) {
    const err = data as ApiError;
    // Preserve distinct code for session probe failures (shell should not treat as logout).
    const msg = err.error?.message ?? `Request failed (${res.status})`;
    const e = new Error(msg) as Error & { code?: string; status?: number };
    e.code = err.error?.code;
    e.status = res.status;
    throw e;
  }
  return data as T;
}

export type Overview = {
  health: {
    score: number;
    max: number;
    factors: { code: string; impact: number; detail: string }[];
  };
  counts: {
    servers: number;
    online: number;
    offline: number;
    warnings: number;
    critical_alerts: number;
    running_containers: number;
    unhealthy_containers: number;
  };
  generated_at: string;
};

export type Server = {
  id: string;
  name: string;
  hostname: string;
  primary_address: string;
  os_name: string;
  os_version: string;
  arch: string;
  status: string;
  health_state: string;
  docker_available: boolean;
  last_seen_at: string | null;
  last_metrics_at?: string | null;
  agent_version?: string | null;
  current_agent_version?: string | null;
  running_containers?: number;
  metrics?: {
    cpu_pct: number | null;
    mem_used_bytes: number | null;
    mem_total_bytes: number | null;
    disk_used_bytes: number | null;
    disk_total_bytes: number | null;
    net_rx_bps: number | null;
    net_tx_bps: number | null;
    uptime_seconds: number | null;
    last_updated: string | null;
  } | null;
};

export type AuthStatus = {
  bootstrap_required: boolean;
  authenticated: boolean;
  user: { id: string; email: string; display_name: string; role: string } | null;
  csrf_token?: string;
};

export const api = {
  authStatus: async () => {
    const data = await request<AuthStatus>("/api/v1/auth/status");
    if (data.csrf_token) setCsrfToken(data.csrf_token);
    return data;
  },
  bootstrap: (email: string, password: string, display_name: string) =>
    request<{ user: AuthStatus["user"]; csrf_token?: string }>("/api/v1/auth/bootstrap", {
      method: "POST",
      body: JSON.stringify({ email, password, display_name }),
    }),
  login: (email: string, password: string) =>
    request<{ user: AuthStatus["user"]; csrf_token?: string }>("/api/v1/auth/login", {
      method: "POST",
      body: JSON.stringify({ email, password }),
    }),
  logout: async () => {
    const data = await request<{ ok: boolean }>("/api/v1/auth/logout", { method: "POST" });
    setCsrfToken(null);
    return data;
  },
  overview: () => request<Overview>("/api/v1/overview"),
  servers: () =>
    request<{ data: Server[]; meta: { total: number; current_agent_version?: string } }>("/api/v1/servers"),
  createServer: (name: string) =>
    request<{ id: string; name: string }>("/api/v1/servers", {
      method: "POST",
      body: JSON.stringify({ name }),
    }),
  createEnrollmentToken: (server_id: string, label: string) =>
    request<{ token: string; expires_at: string; warning: string }>(
      "/api/v1/agents/enrollment-tokens",
      {
        method: "POST",
        body: JSON.stringify({ server_id, label, ttl_minutes: 60, max_uses: 1 }),
      },
    ),
  removeServer: (id: string, opts?: { force?: boolean }) =>
    request<{ ok: boolean; id: string; name: string; mode: string }>(`/api/v1/servers/${id}/remove`, {
      method: "POST",
      body: JSON.stringify({ confirm: true, force: Boolean(opts?.force) }),
    }),
  updateAgent: (id: string, opts?: { channel?: string }) =>
    request<{
      ok: boolean;
      id: string;
      name: string;
      result?: string;
      channel?: string;
      previous_version?: string;
      message?: string;
    }>(`/api/v1/servers/${id}/update-agent`, {
      method: "POST",
      body: JSON.stringify({ confirm: true, channel: opts?.channel }),
    }),
  dockerSummary: () =>
    request<{
      servers: number;
      running: number;
      stopped: number;
      unhealthy: number;
      images: number;
      volumes: number;
      networks: number;
    }>("/api/v1/docker/summary"),
  containers: () =>
    request<{
      data: {
        id: string;
        server_id: string;
        server_name: string;
        container_id: string;
        name: string;
        image_ref: string;
        state: string;
        health: string;
        restart_count: number;
        compose_project: string;
        last_seen_at: string;
      }[];
      meta: { total: number };
    }>("/api/v1/containers"),
  images: () =>
    request<{
      data: {
        id: string;
        server_name: string;
        repository: string;
        tag: string;
        size_bytes: number;
        dangling: boolean;
      }[];
    }>("/api/v1/images"),
  volumes: () =>
    request<{
      data: { name: string; driver: string; server_name: string; unused: boolean }[];
    }>("/api/v1/volumes"),
  networks: () =>
    request<{
      data: { name: string; driver: string; scope: string; server_name: string }[];
    }>("/api/v1/networks"),
  compose: () =>
    request<{
      data: {
        project_name: string;
        status: string;
        server_name: string;
        containers: number;
      }[];
    }>("/api/v1/compose"),
  alerts: () =>
    request<{ data: { id: string; severity: string; status: string; message: string }[] }>(
      "/api/v1/alerts",
    ),
  events: () =>
    request<{ data: { id: string; ts: string; kind: string; severity: string; message: string }[] }>(
      "/api/v1/events",
    ),
};
