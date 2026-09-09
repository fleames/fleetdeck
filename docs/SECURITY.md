# Security model

Security is a first-class requirement. Local-first does not mean lax.

## Threat model (summary)

| Actor | Goal we prevent |
|-------|-----------------|
| Browser XSS | Steal session; trigger management actions |
| Compromised agent | Impersonate other agents; read secrets; mutate other hosts |
| Network attacker | MITM agent↔API; brute enrollment; scrape Docker |
| Curious LAN user | Unauthenticated admin; weak default creds |
| Malicious log content | XSS/HTML execution in log viewer |

## Principles

1. **Agents dial out** — no inbound management ports required on monitored hosts.
2. **Docker socket stays on the host**, used only by the local agent process.
3. **Dashboard never receives** Docker socket, agent secrets, DB passwords, or raw decryptable secret material.
4. **Monitoring is read-only by default**; mutations require authz + confirmation + audit.
5. **Never trust agent payloads** — validate schema, sizes, types, and IDs server-side.
6. **Secrets at rest** — passwords/agent credentials/enrollment tokens are **hashed** (not reversible). The `secrets` table supports AES-GCM envelope encryption keyed from `SESSION_SECRET` for future app secrets; it is empty until a feature calls Put. Agent `credentials.json` on disk is plaintext with mode 0600.
7. **No plaintext passwords** in DB or logs.

## Authentication

### Users

- Password hashing: Argon2id (or bcrypt with strong cost if Argon2 unavailable)
- Sessions: opaque server-side sessions or rotating refresh + short access tokens; prefer **HttpOnly Secure SameSite** cookies for browser
- CSRF: double-submit or synchronizer token for cookie sessions
- Lockout / rate limit on login and enrollment
- No default password in images; first-run bootstrap creates admin interactively

### Agents

```text
Add Server → enrollment token (short-lived, hashed at rest)
  → agent enrolls over TLS
  → receives agent_public_id + agent_secret (shown once / stored hashed)
  → subsequent WSS/HTTPS auth via HMAC or bearer secret
  → rotation endpoint; old secret grace window optional
```

Agents bind to exactly one `server_id` after enrollment. An agent cannot write metrics for another server.

## Authorization

Initial roles (expand later):

| Role | Capabilities |
|------|--------------|
| Admin | Full config, users, tokens, management actions |
| Operator | View + create servers / enrollment + acknowledge/resolve/silence alerts + confirmed management |
| Viewer | Read-only (including personal notification dismiss / dashboard layout) |

Enforce on every mutating route. Frontend hiding is not authorization.

Realtime WebSocket `/api/v1/realtime` requires a signed-in session (not origin-only).

## Docker socket

- **Forbidden:** mounting `/var/run/docker.sock` into publicly exposed API/web containers.
- **Allowed:** agent on host talks to local Docker API with least privilege (dedicated group / rootless where practical).
- Container logs and inspect data flow **agent → API → UI**, sanitized.

## Input / output

- Zod/JSON Schema / Go struct validation at all boundaries
- Parameterized SQL only
- Log viewer: text rendering only; escape HTML; no `dangerouslySetInnerHTML`
- Path parameters constrained; no user-controlled file paths on API host for agent content
- SSRF: API does not fetch arbitrary URLs on behalf of agents/users without allowlists

## Management actions

For start/stop/restart/pull/remove:

1. Explicit control (no hover/drag accidental)
2. Confirmation dialog naming target + consequence
3. Server-side authz + rate limit
4. Audit log entry with result
5. Prefer agent command queue with ack; never expose raw Docker API to browser

## Headers & transport

- TLS in production (terminate at reverse proxy or built-in)
- HSTS when HTTPS
- `Content-Security-Policy`, `X-Content-Type-Options`, `Referrer-Policy`, `Frame-Options`/`frame-ancestors`
- Disable debug endpoints in production builds

## Audit

Record at minimum: login failures/success, enrollment, token create/rotate/revoke, alert ack/resolve/silence, settings changes, any management action, backup/restore.

## Pre-release security review (Phase 8 / DoD)

Checklist includes: authn/z, secrets, Docker access, agent auth, validation, injection, XSS, CSRF, SSRF, path traversal, command execution, log injection, unsafe files, dependency CVEs, default credentials, debug endpoints.

## Explicit non-exposures to frontend

- Agent secrets
- Enrollment token after creation display window (store hash only)
- Database credentials
- Encryption master keys
- Raw Docker engine API
