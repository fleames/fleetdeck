# FleetDeck improvement plan

**Started:** 2026-09-09  
**Source:** [PRODUCTION_AUDIT.md](./PRODUCTION_AUDIT.md)  
**Constraint:** Do not rewrite the stack. Prefer incremental hardening on Go API/agent + Next.js web + Postgres.  
**Goal status:** ACTIVE — P0 none; P1–P2 closed; high-value P3 closed; readiness = READY WITH KNOWN LIMITATIONS ([PRODUCTION_READINESS.md](./PRODUCTION_READINESS.md)). Do not mark goal complete until optional DoD items are accepted or closed.

This plan ranks work **P0 → P4**. Items list concrete next actions. Implementation is a later phase unless noted as a trivial same-pass fix.

---

## P0 — Ship blockers

**None identified in Phase 1 audit** for “fake metrics” or unauthenticated fleet REST reads / Docker shell RCE.

If a P0 appears during implementation (e.g. confirmed session cookie theft on default prod compose), promote it here and stop feature work until fixed.

---

## P1 — Before any shared / internet-exposed deployment

### P1.1 Authenticate realtime WebSocket

- **Why:** `/api/v1/realtime` is origin-checked only; empty Origin allowed; docs claim authenticated WS.
- **Actions:**
  1. Require valid session cookie (or short-lived WS ticket minted after `requireUser`) before `ServeWS`.
  2. Reject empty Origin in production (or always when `COOKIE_SECURE=true`).
  3. Add unit/integration test: no cookie → 401/403; valid session → connect.
  4. Update `docs/API.md` + `SECURITY_REVIEW.md`.

### P1.2 Align AuthZ with SECURITY.md roles

- **Why:** Viewers can create servers and ack/resolve/silence alerts.
- **Actions:**
  1. `POST /servers` → `requireRole("admin","operator")`.
  2. Alert ack/resolve/silence → `requireRole("admin","operator")` (viewer read-only).
  3. Audit any other `requireUser` mutations (compare? dismiss notifications — decide product rule).
  4. Fix `SECURITY_REVIEW.md` “Authz roles on mutating routes” to match reality after patch.
  5. Add role matrix tests.

### P1.3 Historical metrics: use aggregates + honest ranges

- **Why:** History reads raw only (LIMIT 5000); `server_metrics_1h` unused; UI offers `30d` while raw retention defaults to 7d.
- **Actions:**
  1. Implement 1h downsample job; wire `Agg1hRetentionDays` into worker.
  2. History handler: `15m/1h/6h` → raw (or 5m); `24h/7d` → 5m; `30d+` → 1h.
  3. Cap ranges to available retention; return metadata `{source, truncated}`.
  4. Update DoD / DATA_MODEL claims once verified.

### P1.4 Alert engine: duration + cooldown

- **Why:** UI/docs imply sustained breaches; code uses latest sample approximation; cooldown unused.
- **Actions:**
  1. Windowed evaluation over raw (or 5m) for `duration_seconds`.
  2. Enforce `cooldown_seconds` on new fire after resolve.
  3. Optionally honor `scope_type` / `scope_ids`.
  4. Worker unit tests with synthetic metric rows.

### P1.5 Require agent update checksums

- **Why:** Missing SHA256SUMS still allows staging a binary.
- **Actions:**
  1. Fail update if SHA256SUMS missing or artifact unlisted.
  2. Ensure publish scripts always upload SHA256SUMS with channel binaries.
  3. Agent tests for mismatch / missing sums.

### P1.6 Testing baseline

- **Why:** Agent suite empty; web has no automated tests; npm audit advisory-only.
- **Actions:**
  1. ~~Agent: tests for `ContainerAction` allowlist, SHA256 gate, enroll credential write perms.~~
  2. ~~Web: minimal unit smoke for freshness + chart gap helpers (Node test); Playwright E2E still optional.~~
  3. ~~CI: fail on high `pnpm audit` (`--audit-level=high`), not `|| true`.~~
  4. Optional: Compose up + `/healthz` + migrate smoke job.

---

## P2 — Material hardening (production quality)

### P2.1 Secrets at rest

- ~~Use `SESSION_SECRET` (or derived key) for envelope encryption of `secrets` table **or** remove unused table + fix `.env.example` wording until implemented.~~
- ~~Document what is / is not encrypted.~~
- **Done (2026-09-09):** AES-GCM envelope + Store Put/Get; honest docs (hashed vs encrypted); no product writer yet.

### P2.2 Metrics ingest performance

- ~~Batch `COPY` / multi-row INSERT for host + container samples.~~
- ~~Consider BRIN on `ts` or real monthly partitions + partition create job.~~ **Done (2026-09-09):** monthly CREATE for current/next month + DROP of fully-aged named partitions; DELETE remains for DEFAULT/partial months. BRIN still optional.
- **Done (2026-09-09):** multi-row INSERT chunks + concurrent ingest semaphore (503 backpressure).

### P2.3 Agent durable buffer

- ~~On metrics POST failure, spool to disk with size/time caps; flush when API returns.~~
- ~~Surface “buffered / delayed” in agent heartbeat fields for UI freshness.~~
- **Done (2026-09-09):** `metrics-buffer.jsonl` + heartbeat `buffered_samples` / `oldest_buffer_age_sec`.

### P2.4 Retention safety

- ~~Partitioned drop vs bulk DELETE on DEFAULT.~~ **Done (hybrid 2026-09-09):** ensure monthly partitions; DROP fully-aged months; DELETE for DEFAULT + aggs.
- ~~Retain 1h table; document restore = `pg_dump` volume, not Settings JSON alone.~~
- **Done (2026-09-09):** Settings retention wired into worker; MONITORING/DATABASE honesty.

### P2.5 Rate limit durability

- ~~Persist or share login/enroll limiters if API is multi-instance later.~~ **Done (2026-09-09):** `rate_limit_buckets` table + memory fallback. Redis not required for single/small multi-API.
- ~~Bind to CF-Connecting-IP when behind Tunnel.~~ **Done (2026-09-09).**

### P2.6 Docs honesty pass

- ~~ARCHITECTURE: HTTPS bearer (not mTLS) unless mTLS added.~~
- DEFINITION_OF_DONE: mark aggregation/history/authz only after P1 evidence. *(P1 already done; leave DoD for readiness pass)*
- API.md realtime auth wording. *(verify if stale)*
- **Done (2026-09-09):** MONITORING.md + DATABASE.md created; TROUBLESHOOTING/ARCHITECTURE/SECURITY/DATA_MODEL/.env.example updated.

---

## P3 — Scale, UX, polish

### P3.1 Frontend design-system alignment

- ~~Shared chart wrapper (theme-aware ECharts).~~ (`MetricsChart`)
- ~~Sidebar live counts (alerts / unhealthy / offline) from overview.~~
- ~~Stronger loading vs empty vs error primitives across list pages.~~ (page-state + inventory/events/docker/metrics/topology)
- ~~Chart gap handling (connectNulls false + null inserts).~~
- ~~Dashboard LIVE / RECENT / STALE / OFFLINE freshness clarity.~~
- ~~Docker inventory tables: search/filter + linked summary tiles.~~
- ~~Remove unused `PlaceholderPage`.~~
- ~~Accessibility basics: skip link, main landmark, nav `aria-current` / labels.~~

### P3.2 Topology / dashboard customization

- ~~Topology inventory map polish (StatusPill, summary tiles, load/error/empty).~~
- Incremental toward UI.md free-form layout persistence — **deferred** (widget show/hide only).

### P3.3 Alert scope + notification channels

- ~~Scope rules to server IDs (`scope_type=servers` + `scope_ids`).~~ **Done (2026-09-09).**
- ~~Optional webhook channel (still local-first).~~ **Done (2026-09-09):** `ALERT_WEBHOOK_URL` or `settings.alerts.webhook_url` on fire/resolve.
- Email/Discord/Slack still deferred.

### P3.4 Events/audit reliability

- ~~Events page error state.~~
- Fail loudly on audit insert errors in debug; metrics for dropped audits. **Deferred.**

### P3.5 OpenAPI / shared contracts

- Populate `packages/shared` or drop references until real. **Deferred.**

### P3.6 WS origin policy + multiplexing topics

- ~~Tighten CheckOrigin with session auth (P1.1).~~
- Authz-scoped topics later. **Deferred.**

---

## P4 — Nice-to-have / cleanup

- ~~Document `api_tokens` as planned/not implemented in DATA_MODEL.~~
- Implement or remove `api_tokens` table/API. **Deferred.**
- GPG-signed release SHA256SUMS (DoD optional).
- i18n catalogs (English-first structure).
- Free-form drag dashboard grid.
- Timescale / dedicated TSDB only if Postgres partitions prove insufficient.

---

## Suggested implementation sequence (next phases)

| Phase | Focus | Exit criteria |
|-------|--------|---------------|
| **1 (this turn)** | Audit + plan | `PRODUCTION_AUDIT.md` + this file exist |
| **2** | P1.1 + P1.2 security auth | WS session required; role matrix matches SECURITY.md; tests green |
| **3** | P1.3 + P1.4 data truth | History uses aggs; duration/cooldown real; DoD updated |
| **4** | P1.5 + P1.6 | Checksums mandatory; agent+web CI tests |
| **5** | P2 cluster | Secrets story, ingest batch, buffer, docs honesty |
| **6** | P3 UX + readiness verification | Compose smoke, backup drill, SECURITY_REVIEW re-check |
| **7** | Release gates | Only then consider marking production goal complete |

---

## Explicitly deferred (do not do in Phase 1)

- Stack rewrite / replace Next or Go.
- Forking PulseOps.
- Implementing all P2–P4 in one pass.
- Marking the production goal complete.

---

## Tracking

| ID | Item | Status |
|----|------|--------|
| P1.1 | Authenticate realtime WS | **done** (2026-09-09) |
| P1.2 | AuthZ role alignment | **done** (2026-09-09) |
| P1.3 | Metrics history + 1h pipeline | **done** (2026-09-09) |
| P1.4 | Alert duration/cooldown | **done** (2026-09-09) |
| P1.5 | Mandatory update checksums | **done** (2026-09-09) |
| P1.6 | Agent/web/CI tests | **done** (2026-09-09) — agent allowlist/SHA256/creds; web format+gap unit tests; CI audit fails on high; Playwright smoke + CI e2e job |
| P2.1 | Secrets at rest / envelope | **done** (2026-09-09) — AES-GCM store + honest docs; no product Put yet |
| P2.2 | Metrics ingest batch + backpressure | **done** (2026-09-09) — multi-row INSERT + semaphore; monthly partitions create/drop |
| P2.3 | Agent durable buffer | **done** (2026-09-09) |
| P2.4 | Retention safety | **done** (2026-09-09) — DROP aged months + DELETE DEFAULT/aggs |
| P2.5 | Rate limit durability | **done** (2026-09-09) — CF-Connecting-IP + Postgres buckets |
| P2.6 | Docs honesty | **done** (2026-09-09) — MONITORING.md, DATABASE.md, ARCHITECTURE/SECURITY/TROUBLESHOOTING |
| P3.* | UX / scale | **mostly done** (2026-09-09) — topology/a11y/page-state; alert scope+webhook done; OpenAPI/drag-grid deferred |
| P4.* | Cleanup | **partial** — api_tokens honesty in DATA_MODEL; GPG/i18n/drag-grid deferred |
| Ready | Production readiness report | **READY WITH KNOWN LIMITATIONS** — see [PRODUCTION_READINESS.md](./PRODUCTION_READINESS.md); goal remains ACTIVE |
