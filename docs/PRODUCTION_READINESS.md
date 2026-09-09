# FleetDeck production readiness

**Date:** 2026-09-09  
**Goal status:** ACTIVE  
**Readiness verdict:** **READY WITH KNOWN LIMITATIONS**

This report is an honest gate after Phase 1 audit + P1–P3 hardening + follow-on limitation closures. It is **not** a claim that every Definition of Done row is perfect or that FleetDeck is HA / multi-region ready.

---

## Verdict

| Field | Value |
|-------|--------|
| Status string | `READY WITH KNOWN LIMITATIONS` |
| Suitable for | Local-first / home-lab / small private fleet behind TLS (e.g. Cloudflare Tunnel) |
| Not suitable for yet | Multi-API HA without sticky ops expectations, untrusted public internet without edge TLS, compliance-grade audit durability, free-form drag dashboards |

Core product paths (enroll → agent metrics → alerts → Docker actions → dashboard) are real, authenticated, and tested at unit + browser smoke level. Remaining gaps are scale polish, optional features, and operational maturity—not fake metrics or open unauthenticated fleet REST.

---

## What was audited

Source: [PRODUCTION_AUDIT.md](./PRODUCTION_AUDIT.md) (2026-09-09 code/config inspection).

Covered: architecture, data integrity, agent security, Docker actions, AuthN/AuthZ, DB/metrics/history, alerts, events/audit, frontend UX, performance, reliability, SECURITY.md accuracy, CI/tests, deploy/backup docs.

**Fake metrics search:** no production UI/API path invents CPU/RAM/Docker samples.

---

## What changed / fixed (P1–P3 + follow-on)

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
| P2.2 | Multi-row metrics INSERT + ingest backpressure; monthly partition create/drop job |
| P2.3 | Agent durable metrics buffer + heartbeat buffer fields |
| P2.4 | Settings retention → worker; DROP aged monthly partitions + DELETE fallback |
| P2.5 | CF-Connecting-IP keying; Postgres `rate_limit_buckets` for login/enroll (memory fallback) |
| P2.6 | MONITORING.md, DATABASE.md, architecture/security honesty pass |

### P3 (UX / polish) — high-value incremental

| Item | Status |
|------|--------|
| Shared `MetricsChart`, chart gap helpers, freshness labels | done |
| Sidebar live alert/unhealthy/offline counts | done |
| Loading / empty / error primitives (`page-state`) on list pages | done |
| Topology: live inventory map + summary tiles + StatusPill + load/error | done |
| Metrics page aligned with page-state + StatusPill | done |
| Remove unused `PlaceholderPage` | done |
| A11y basics: skip link, main landmark, nav `aria-current`, labels | done |
| Alert scope (`scope_type`/`scope_ids`) in evaluator | **done** (this pass) |
| Webhook channel on fire/resolve (`ALERT_WEBHOOK_URL` / settings) | **done** (this pass) |
| OpenAPI / `packages/shared` | **not done** (empty / deferred) |
| Free-form drag dashboard grid | **not done** (widget show/hide only) |

### Follow-on (2026-09-09 limitation pass)

| Item | Evidence |
|------|----------|
| Expanded `/healthz` | DB ping + online agents/servers + ingest last-hour + worker last-run |
| Self-metrics worker/webhook fields | `/api/v1/overview/self` |
| Playwright browser smoke | `apps/web/e2e/smoke.spec.ts`; CI `e2e` job (compose up → bootstrap → overview) |

---

## What was tested (this readiness pass)

| Check | Result |
|-------|--------|
| `go test ./...` in `apps/api` | pass |
| `go test ./...` in `apps/agent` | pass (prior + unchanged paths) |
| `pnpm test` in `apps/web` | pass (unit) |
| `pnpm lint` in `apps/web` | pass (prior) |
| Playwright smoke (CI job) | wired: bootstrap/login → `overview-dashboard` |
| Compose `api` `/healthz` | expanded JSON (`status`, `database`, `fleet`, `ingest`, `worker`) |
| GPG-signed release SHA256SUMS | **not done** (optional) |

---

## What remains / known limitations

1. **Single API process** — ingest + workers + WS in one binary; restart drops WS (polls compensate).
2. **Control-plane SPOF** — remote agents depend on reachable `API_PUBLIC_URL` / Tunnel host.
3. **Metrics storage** — monthly partitions created for current/next month; aged named partitions DROP; **DEFAULT** partition still uses DELETE for leftover/historical rows; BRIN optional later.
4. **Rate limits** — login/enroll counters in Postgres (`rate_limit_buckets`); fall back to in-memory if DB write fails. Not Redis; no global API rate limit beyond those endpoints.
5. **Secrets product path** — envelope crypto exists; no Settings UI writer for arbitrary secrets yet.
6. **Alert channels** — webhook fire/resolve done; email/Discord/Slack still not implemented (local-first).
7. **Audit durability** — audit inserts remain best-effort (`_ = Exec` style); no dropped-audit metrics.
8. **Topology** — inventory hierarchy, not a rich interactive map; no free-form dashboard grid.
9. **Contracts** — `packages/shared` empty; no OpenAPI codegen.
10. **`api_tokens`** — documented as planned only; session cookies are dashboard auth.
11. **TLS** — terminated at reverse proxy / Tunnel; Compose itself is HTTP on localhost.
12. **Tests** — Playwright smoke in CI; no full DB integration suite / multi-browser matrix.
13. **Agent credentials on disk** — plaintext JSON at 0600 (host compromise = that host’s agent identity).

---

## Definition of Done cross-check

See [DEFINITION_OF_DONE.md](./DEFINITION_OF_DONE.md). Most product rows are **done** or **mostly**. Open items that keep the goal **ACTIVE**:

- Optional GPG signing of release `SHA256SUMS`.
- P3 leftovers (OpenAPI / drag grid) and intentional acceptance of SPOF / DEFAULT DELETE residual.

**Do not mark the production goal COMPLETE** until you intentionally accept the limitations above (or close them). This report supports **READY WITH KNOWN LIMITATIONS** while the goal stays ACTIVE.

---

## Operator checklist before shared use

1. Rotate Compose / `.env` secrets; set strong `SESSION_SECRET`, DB password, admin password.
2. Put API/web behind HTTPS (`COOKIE_SECURE=true`).
3. Confirm CDN publishes `SHA256SUMS` with every agent channel binary.
4. Prefer `pg_dump` of the Postgres volume for DR (Settings backup ≠ full restore of servers/metrics).
5. Re-run `/healthz` + login smoke after deploy.
6. Optional: set `ALERT_WEBHOOK_URL` (or `settings.alerts.webhook_url`) for ops notifications.
