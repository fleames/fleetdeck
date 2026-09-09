# FleetDeck production readiness

**Date:** 2026-09-09  
**Goal status:** COMPLETE  
**Readiness verdict:** **READY WITH KNOWN LIMITATIONS**  
**GOAL_COMPLETE:** yes

This report is the final gate after Phase 1 audit + P1–P3 hardening + follow-on closures. It is **not** a claim of bare `READY`, HA / multi-region readiness, or perfect Definition of Done coverage. Remaining gaps below are **intentionally accepted** product limits or **Future** work.

---

## Final verdict

| Field | Value |
|-------|--------|
| Status string | `READY WITH KNOWN LIMITATIONS` |
| Goal deliverables | `PRODUCTION_AUDIT.md`, `IMPROVEMENT_PLAN.md`, incremental P1–P3 fixes shipped, this report |
| Suitable for | Local-first / home-lab / small private fleet behind TLS (e.g. Cloudflare Tunnel) |
| Not suitable for yet | Multi-API HA, untrusted public internet without edge TLS, compliance-grade audit durability, free-form drag dashboards |

Core product paths (enroll → agent metrics → alerts → Docker actions → dashboard) are real, authenticated, and covered by unit tests plus Playwright smoke in CI. Do **not** claim bare `READY` while SPOF and DEFAULT-partition residual remain by design.

---

## Accepted product limits (local-first single-node)

These are **not** blockers for `READY WITH KNOWN LIMITATIONS`. They are the product shape until an HA redesign:

1. **Single API process (control-plane SPOF)** — ingest, workers, and WS share one binary; restart drops WS (polls compensate). Remote agents depend on a reachable `API_PUBLIC_URL` / Tunnel host. Home-lab / single-node by design.
2. **DEFAULT partition residual** — monthly partitions CREATE for current/next month; aged named partitions DROP; leftover / historical rows on **DEFAULT** still use DELETE. BRIN / full partition-only retention is Future.

---

## Future work (non-blocking / P4+)

| Item | Notes |
|------|--------|
| Alert email (SMTP) | Discord/Slack-compatible webhooks done; email deferred |
| OpenAPI / `packages/shared` | Package empty; no codegen |
| Free-form drag dashboard grid | Widget show/hide only |
| GPG-signed release `SHA256SUMS` | Checksums mandatory; GPG optional |
| Global API rate limit / Redis | Login/enroll Postgres buckets + memory fallback |
| Compliance audit bus | `CRITICAL` log on insert fail; still best-effort |
| Full DB integration suite / multi-browser E2E | Playwright smoke wired in CI |

---

## What was audited

Source: [PRODUCTION_AUDIT.md](./PRODUCTION_AUDIT.md) (2026-09-09).

Covered: architecture, data integrity, agent security, Docker actions, AuthN/AuthZ, DB/metrics/history, alerts, events/audit, frontend UX, performance, reliability, SECURITY.md accuracy, CI/tests, deploy/backup docs.

**Fake metrics search:** no production UI/API path invents CPU/RAM/Docker samples.

---

## What changed / fixed (P1–P3 + follow-on)

### P1 — closed

| ID | Fix |
|----|-----|
| P1.1 | Realtime WS requires session; empty Origin rejected when secure cookies |
| P1.2 | Viewer cannot create servers or mutate alerts; role matrix tests |
| P1.3 | History uses raw/5m/1h by range; 1h downsample worker; retention metadata |
| P1.4 | Alert `duration_seconds` windowed; `cooldown_seconds` enforced |
| P1.5 | Agent update fails closed without SHA256SUMS |
| P1.6 | Agent + web unit tests; CI `pnpm audit` fails on high |

### P2 — closed

| ID | Fix / note |
|----|------------|
| P2.1 | AES-GCM secrets envelope + admin PUT/DELETE + Settings signing-secret UI |
| P2.2 | Multi-row metrics INSERT + ingest backpressure; monthly partition create/drop job |
| P2.3 | Agent durable metrics buffer + heartbeat buffer fields |
| P2.4 | Settings retention → worker; DROP aged monthly partitions + DELETE fallback |
| P2.5 | CF-Connecting-IP keying; Postgres `rate_limit_buckets` for login/enroll |
| P2.6 | MONITORING.md, DATABASE.md, architecture/security honesty pass |

### P3 — high-value closed; polish leftovers → Future

Shared charts, sidebar counts, page-state, topology polish, a11y basics, alert scope, Discord/Slack webhooks + Settings. OpenAPI and drag grid deferred (see Future).

### Follow-on evidence

Expanded `/healthz` (DB + fleet + ingest + worker); Playwright `apps/web/e2e/smoke.spec.ts` + CI `e2e`; secrets product path; audit `CRITICAL:` on insert failure; agent creds `umask`/`chmod` 0700/0600; Compose TLS via proxy/Tunnel (documented).

---

## Evidence (this finalization pass — 2026-09-09)

| Check | Result |
|-------|--------|
| `go test ./...` in `apps/api` | **pass** |
| `go test ./...` in `apps/agent` | **pass** |
| `pnpm test` in `apps/web` | **pass** (3 freshness/gap unit tests) |
| Playwright smoke | wired in CI (`e2e` job); bootstrap/login → `overview-dashboard` |
| Compose runtime | `db`/`api`/`web` Up; web `/` → **200** |
| `GET http://127.0.0.1:8080/healthz` | **200** expanded JSON: `database=ok`, fleet online counts, ingest last-hour points, worker last-run timestamps |

---

## Definition of Done cross-check

See [DEFINITION_OF_DONE.md](./DEFINITION_OF_DONE.md). Product rows are **done** or **mostly** with intentional acceptance of local-first limits and Future P4 items. Remaining open rows (GPG, OpenAPI, drag grid, email) do **not** block this verdict.

---

## Operator checklist before shared use

1. Rotate Compose / `.env` secrets; set strong `SESSION_SECRET`, DB password, admin password.
2. Put API/web behind HTTPS (`COOKIE_SECURE=true`) — not raw Compose ports on WAN.
3. Confirm CDN publishes `SHA256SUMS` with every agent channel binary.
4. Prefer `pg_dump` of the Postgres volume for DR (Settings backup ≠ full restore of servers/metrics).
5. Re-run `/healthz` + login smoke after deploy.
6. Optional: set alert webhook in Settings (`alerts.webhook_url`) or `ALERT_WEBHOOK_URL`; optional signing secret under Settings → Webhook signing secret.
