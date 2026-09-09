# Security review checklist (Phase 8)

Status as of hardening + CI pass. Items marked **done** have implementing code + verification notes.

| Area | Status | Evidence / notes |
|------|--------|------------------|
| Authn (Argon2id, HttpOnly SameSite cookies) | **done** | `internal/auth` |
| Authz roles on mutating routes | **done** | `requireRole(admin,operator)` for server create/remove, alert ack/resolve/silence, container actions, tokens; admin-only settings/users/backup; viewers read-only |
| Authenticated realtime WebSocket | **done** | Session cookie required; empty Origin rejected when `COOKIE_SECURE=true` |
| Unauthenticated fleet reads blocked | **done** | Fleet routes use `requireUser` |
| Login rate limit + failed login audit | **done** | DB `rate_limit_buckets` (+ memory fallback), audit `auth.login` failed |
| Enrollment rate limit | **done** | Same shared limiter on `/agent/v1/enroll` |
| Agent body size limits | **done** | `maxBody` on agent POST routes |
| JSON decode size cap | **done** | `httpx.Decode` LimitReader 1 MiB |
| Management confirm + audit | **done** | `confirm=true` + audit on container actions |
| Env mask + admin reveal audit | **done** | `/containers/{id}/env` |
| Security headers | **done** | nosniff, DENY frame, CSP for API JSON |
| HSTS when HTTPS cookies | **done** | Emitted when `COOKIE_SECURE=true` |
| Docker socket not in API/web images | **done** | Compose agent-on-host model; no sock mounts in API/web Dockerfiles |
| Log XSS (text-only render) | **done** | Logs UI + container detail use text nodes |
| CSRF synchronizer token | **done** | Cookie + `X-CSRF-Token`; web `apiFetch` |
| Dependency CVE scan CI | **done** | `govulncheck` in `.github/workflows/ci.yml`; `scripts/scan-deps.sh` |
| Multi-user operator/viewer UX | **done** | Admin Users page create/list; API roles enforced |
| Failure-path tests | **mostly** | CSRF/confirm/validation/rate-limit unit tests + CI |
| TLS termination in Compose | **documented** | Reverse proxy + `COOKIE_SECURE`; not baked into local compose |

## Residual risks

- Local `.env` `SESSION_SECRET` is a development placeholder — rotate before any shared deployment. It derives the AES-GCM key for the `secrets` table (sessions remain opaque DB-hashed tokens).
- Management actions depend on a healthy agent command poll loop; timeouts return 202 accepted.
- `pnpm audit` in CI is advisory (`|| true`) — review output on each release.
