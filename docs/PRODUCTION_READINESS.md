# FleetDeck production readiness

**Date:** 2026-09-09  
**Goal status:** ACTIVE  
**Readiness verdict:** **READY WITH KNOWN LIMITATIONS**

This report is an honest gate after Phase 1 audit + P1–P3 hardening. It is **not** a claim that every Definition of Done row is perfect or that FleetDeck is HA / multi-region ready.

---

## Verdict

| Field | Value |
|-------|--------|
| Status string | `READY WITH KNOWN LIMITATIONS` |
| Suitable for | Local-first / home-lab / small private fleet behind TLS (e.g. Cloudflare Tunnel) |
| Not suitable for yet | Multi-API HA, untrusted public internet without edge TLS, compliance-grade audit durability, free-form drag dashboards |

Core product paths (enroll → agent metrics → alerts → Docker actions → dashboard) are real, authenticated, and tested at unit level. Remaining gaps are scale, polish depth, optional features, and operational maturity—not fake metrics or open unauthenticated fleet REST.

---

## What was audited

Source: [PRODUCTION_AUDIT.md](./PRODUCTION_AUDIT.md) (2026-09-09 code/config inspection).

Covered: architecture, data integrity, agent security, Docker actions, AuthN/AuthZ, DB/metrics/history, alerts, events/audit, frontend UX, performance, reliability, SECURITY.md accuracy, CI/tests, deploy/backup docs.

**Fake metrics search:** no production UI/API path invents CPU/RAM/Docker samples.

---

## What changed / fixed (P1–P3)

### P1 (ship blockers for shared exposure) — closed

| ID | Fix |
|----|-----|
| P1.1 | Realtime WS requires session; empty Origin rejected when secure cookies |
| P1.2 | Viewer cannot create servers or mutate alerts; role matrix tests |
| P1.3 | History uses raw/5m/1h by range; 1h downsample worker; retention metadata |
| P1.4 | Alert `duration_seconds` windowed; `cooldown_seconds` enforced |
| P1.5 | Agent update fails closed without SHA256SUMS |
| P1.6 | Agent + web unit tests; CI `pnpm audit` fails on high |

### P2 (material hardening) — largely closed

| ID | Fix / note |
|----|------------|
| P2.1 | AES-GCM secrets envelope + honest docs (no product Put yet) |
| P2.2 | Multi-row metrics INSERT + ingest backpressure (BRIN/partitions deferred) |
| P2.3 | Agent durable metrics buffer + heartbeat buffer fields |
| P2.4 | Settings retention → worker; DELETE (not partition DROP) documented |
| P2.5 | CF-Connecting-IP rate-limit keying; shared/persist limiter deferred |
| P2.6 | MONITORING.md, DATABASE.md, architecture/security honesty pass |

### P3 (UX / polish) — high-value incremental

| Item | Status |
|------|--------|
| Shared `MetricsChart`, chart gap helpers, freshness labels | done |
| Sidebar live alert/unhealthy/offline counts | done |
| Loading / empty / error primitives (`page-state`) on list pages | done |
| Topology: live inventory map + summary tiles + StatusPill + load/error | done (this pass) |
| Metrics page aligned with page-state + StatusPill | done (this pass) |
| Remove unused `PlaceholderPage` | done (this pass) |
| A11y basics: skip link, main landmark, nav `aria-current`, labels | done (this pass) |
| Alert scope / webhook channels | **not done** (deferred) |
| OpenAPI / `packages/shared` | **not done** (empty / deferred) |
| Free-form drag dashboard grid | **not done** (widget show/hide only) |

---

## What was tested (this readiness pass)

| Check | Result |
|-------|--------|
| `go test ./...` in `apps/api` | pass |
| `go test ./...` in `apps/agent` | pass |
| `pnpm test` in `apps/web` | pass (3 unit tests) |
| `pnpm lint` in `apps/web` | pass |
| Compose `api` `/healthz` | `200` `{"status":"ok"}` |
| Compose `web` `/` | `200` |
| Full browser E2E (login → dashboard → servers) | **not automated** |
| GPG-signed release SHA256SUMS | **not done** (optional) |

---

## What remains / known limitations

1. **Single API process** — ingest + workers + WS in one binary; restart drops WS (polls compensate).
2. **Control-plane SPOF** — remote agents depend on reachable `API_PUBLIC_URL` / Tunnel host.
3. **Metrics storage** — DEFAULT partition only; retention via DELETE; BRIN/monthly partitions deferred.
4. **Rate limits** — in-memory (per process); reset on restart; not shared across API replicas.
5. **Secrets product path** — envelope crypto exists; no Settings UI writer for arbitrary secrets yet.
6. **Alert scope / webhooks** — rules are fleet-wide; notifications are in-app only.
7. **Audit durability** — audit inserts remain best-effort (`_ = Exec` style); no dropped-audit metrics.
8. **Topology** — inventory hierarchy, not a rich interactive map; no free-form dashboard grid.
9. **Contracts** — `packages/shared` empty; no OpenAPI codegen.
10. **`api_tokens`** — documented as planned only; session cookies are dashboard auth.
11. **TLS** — terminated at reverse proxy / Tunnel; Compose itself is HTTP on localhost.
12. **Tests** — no Playwright/browser smoke in CI; no full DB integration suite.
13. **Agent credentials on disk** — plaintext JSON at 0600 (host compromise = that host’s agent identity).

---

## Definition of Done cross-check

See [DEFINITION_OF_DONE.md](./DEFINITION_OF_DONE.md). Most product rows are **done** or **mostly**. Open items that keep the goal **ACTIVE**:

- Prefer browser smoke automation for release confidence.
- Optional GPG signing of release `SHA256SUMS`.
- P3 leftovers (scope/webhooks/OpenAPI) and P4 cleanup are not ship blockers for local-first production use.

**Do not mark the production goal COMPLETE** until you intentionally accept the limitations above (or close them). This report supports **READY WITH KNOWN LIMITATIONS** while the goal stays ACTIVE.

---

## Operator checklist before shared use

1. Rotate Compose / `.env` secrets; set strong `SESSION_SECRET`, DB password, admin password.
2. Put API/web behind HTTPS (`COOKIE_SECURE=true`).
3. Confirm CDN publishes `SHA256SUMS` with every agent channel binary.
4. Prefer `pg_dump` of the Postgres volume for DR (Settings backup ≠ full restore of servers/metrics).
5. Re-run `/healthz` + login smoke after deploy.
