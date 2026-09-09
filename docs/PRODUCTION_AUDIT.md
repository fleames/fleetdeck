# FleetDeck production audit

**Date:** 2026-09-09  
**Scope:** Current monorepo (`apps/web`, `apps/api`, `apps/agent`, migrations, Compose, auth, metrics, alerts, CI, docs).  
**Method:** Code and config inspection only (no stack rewrite).  
**Goal status:** ACTIVE — this audit is Phase 1 evidence for hardening toward production readiness.

Severity scale: **P0** ship-blocker / active exploit or data lie · **P1** high risk before shared/prod use · **P2** material gap · **P3** polish / scale · **P4** nice-to-have / docs drift.

---

## Executive summary

FleetDeck is a real agent→API→Postgres→Next.js stack with enrollment, hashed credentials, confirmed Docker actions, Argon2id sessions, CSRF on mutations, and no discovered fake/demo metric generators in production UI paths. Remaining production gaps cluster around **authz completeness vs SECURITY.md**, **unauthenticated realtime**, **incomplete metrics aggregation/history**, **alert duration/cooldown not enforced**, **secrets-at-rest unused**, and **thin automated test coverage** (especially agent + web).

**Fake metrics search:** `mock` / `faker` / `Math.random` / hardcoded CPU-RAM demo paths were searched under `apps/`. Matches are test fakes (`handlers_ops_test.go`), empty-state copy asserting “never mock data”, and UI `placeholder=` attributes — **not** fabricated fleet metrics.

---

## 1. Architecture

### Components (as implemented)

| Component | Path | Role |
|-----------|------|------|
| Dashboard | `apps/web` | Next.js App Router UI; same-origin `/api` rewrite preferred |
| Monitoring API | `apps/api` | Chi HTTP, session auth, agent ingest, workers, WS hub |
| Agent | `apps/agent` | Dial-out collectors (host + Docker Engine API), command poll |
| Database | Postgres 16 via Compose | Registry, inventory, metrics, alerts, audit |
| Edge | Cloudflare Tunnel (`cloudflared` profile) + optional relay | Remote agent reachability |
| CDN | R2 / `AGENT_CDN_BASE` | `install.sh` + agent binaries + SHA256SUMS |

### Data flows

1. Operator creates server + enrollment token → install script/CDN one-liner.  
2. Agent enrolls over HTTPS → stores `credentials.json` (0600) → heartbeats / metrics / inventory.  
3. API binds payloads to `agentIdentity.ServerID` from credentials (not client-supplied server id).  
4. Worker marks offline, evaluates alerts, retains/downsamples raw→5m.  
5. UI polls REST + multiplexed WS for refresh triggers.

### Coupling & SPOFs

| Finding | Sev | Evidence | Current behavior | Risk |
|---------|-----|----------|------------------|------|
| Single API process owns ingest + workers + WS | P2 | `apps/api/cmd/fleetdeck-api/main.go` | One binary; workers started in-process | API restart loses in-flight WS; no HA |
| Tunnel / `API_PUBLIC_URL` SPOF for remote agents | P2 | `deploy/docker-compose.yml`, `docs/REMOTE_AGENTS.md` | Agents dial public URL; home PC offline → agents fail | Fleet appears offline when LAN control plane down |
| Architecture docs describe WSS/mTLS & OpenAPI shared package | P3 | `docs/ARCHITECTURE.md` diagram; empty `packages/shared/` | Agent uses HTTPS JSON + bearer; no mTLS; no OpenAPI codegen | Doc/implementation inconsistency; contract drift |
| Docs claim authenticated realtime | P1 | `docs/API.md` “authenticated WebSocket”; `server.go` `GET /api/v1/realtime` without `requireUser` | WS only checks Origin allowlist (empty Origin allowed) | Unauthenticated listeners get fleet event fan-out |

---

## 2. Data integrity (real vs empty / loading / error)

| Finding | Sev | Evidence | Current behavior | Risk |
|---------|-----|----------|------------------|------|
| No fake production metrics generators found | — | Repo grep; collectors in `apps/agent/internal/collect/host.go`, `docker.go` | Metrics from gopsutil / Docker API | Positive: integrity goal honored |
| Empty states are explicit, not fabricated | — | `apps/web/src/app/page.tsx`, `docker/page.tsx`, `placeholder-page.tsx` | Copy states empty ≠ mock | Good |
| Health score from live DB counts | — | `handlers_auth.go` `healthScore` | Derived from online/offline/alerts/unhealthy | Real; not random |
| Loading/error separation uneven | P3 | Many pages use `error` banners; some show `—` while `null` self-metrics | Partial loading UX | Operators may confuse “loading” with “zero” |
| `PlaceholderPage` unused leftover | P4 | Only defined in `placeholder-page.tsx` | Dead component | Mild confusion vs “coming online” |

---

## 3. Server agent security & reliability

| Finding | Sev | Evidence | Current behavior | Risk |
|---------|-----|----------|------------------|------|
| Enrollment tokens + agent secrets hashed | — | `enrollment_tokens.token_hash`, `agent_credentials.secret_hash`; Argon2id via `HashSecret` | Secrets not stored plaintext in DB | Good |
| Agent bound to one `server_id` | — | `requireAgent` joins credentials→agent→server; ingest uses `ident.ServerID` | Cannot write another host’s metrics | Good |
| Credentials on disk plaintext JSON | P2 | `main.go` `credentials.json` mode 0600 | File readable by root/agent user | Host compromise = agent impersonation of that host only |
| Agent update SHA256 optional | P1 | `update.go` `fetchSHA256For`: missing SHA256SUMS → `ok=false`, still stages binary | Unsigned CDN channel accepted | Supply-chain / CDN tamper if sums absent |
| Agent has **zero** Go tests | P1 | No `*_test.go` under `apps/agent` | CI builds only | Regressions in Docker actions/update/enroll undetected |
| Command types allowlisted in agent | — | `main.go` command switch + `collect.ContainerAction` | Unknown types fail; Docker via Engine HTTP not shell | Good — no shell injection on lifecycle paths |
| Uninstall/update via fixed systemd path units | — | `uninstall.go`, `update.go` `exec.Command("systemctl", …)` | Fixed argv, not user strings | Good |

---

## 4. Docker management safety

| Finding | Sev | Evidence | Current behavior | Risk |
|---------|-----|----------|------------------|------|
| Authz on container actions | — | `server.go` `requireRole("admin","operator")` | Viewers blocked | Good |
| `confirm=true` required | — | `handlers_actions.go`; UI confirm dialog `containers/[id]/page.tsx` | Missing confirm → `confirmation_required` | Good |
| Audit + infrastructure event on queue | — | `handlers_actions.go` | Logged with user email / command id | Good |
| Action set is allowlisted | — | `allowedContainerActions` + agent switch | No arbitrary Docker API from browser | Good |
| No shell concatenation of container IDs into bash | — | `collect/actions.go` HTTP paths | IDs in URL path to Docker Engine | Residual: malformed IDs rejected by Engine |
| Install one-liners escape tokens | — | `handlers_cdn_install.go` `escToken` / `'\''` | Shell metachar defense | Good |
| Env reveal admin-only + audited | — | `handleContainerEnv` | Masked by default | Good |
| Destructive remove/update require confirm (+ force offline) | — | `handlers_fleet.go` | Conflict when agent offline without force | Good |

---

## 5. AuthN / AuthZ

| Finding | Sev | Evidence | Current behavior | Risk |
|---------|-----|----------|------------------|------|
| Argon2id passwords; opaque hashed sessions; HttpOnly cookies | — | `auth/service.go`, `crypto.go` | Matches SECURITY.md authn | Good |
| CSRF double-submit (cookie + `X-CSRF-Token`) | — | `csrf.go`; web `api.ts` `ensureCsrfToken` | Mutations blocked without match | Good; login/bootstrap exempt |
| Login + enroll rate limits | — | `security.go` 20/min login, 30/min enroll | In-memory, per-IP | Soft; resets on restart |
| **Viewer can mutate alerts** | P1 | `POST /alerts/{id}/acknowledge|resolve|silence` → `requireUser` only | Contradicts SECURITY.md “Viewer: Read-only” | Privilege creep |
| **Viewer can create servers** | P1 | `POST /servers` → `requireUser` | Any authenticated role enrolls fleet surface | Unwanted registry growth |
| SECURITY_REVIEW claims “roles on mutating routes” **done** | P1 | `docs/SECURITY_REVIEW.md` vs above routes | Overstates completeness | False confidence |
| Realtime WS unauthenticated | P1 | `server.go` L55; `hub.go` origin only | Event leakage (activity signals) | Recon / privacy |
| `SESSION_SECRET` required but unused | P2 | `config.go` length check only; no encrypt/HMAC usage | Misleading `.env.example` (“signing and secret encryption”) | Operators think envelope encryption exists |
| `secrets` table unused | P2 | `001_init.sql`; no Go INSERT/SELECT | Encryption-at-rest principle incomplete | Future secret storage risk if bolted on later |
| No lockout beyond rate limit | P3 | Login limiter only | Brute force slowed, not locked | Acceptable for local-first; weak on exposed WAN API |

---

## 6. Database

| Finding | Sev | Evidence | Current behavior | Risk |
|---------|-----|----------|------------------|------|
| Core schema with FKs + CASCADE | — | `001_init.sql` | Servers→agents/inventory/metrics cascade | Good |
| Metrics “partitioned” but only DEFAULT partition | P2 | `server_metrics_raw` + `*_default PARTITION` | No monthly partition maintenance | All raw data in one partition → bloat/vacuum cost |
| Indexes on status, sessions, alerts, events, audit | — | `001_init.sql` | Basic query support | OK for small fleets |
| Retention deletes raw + 5m; downsamples 5m from last 2h | P2 | `worker/runner.go` `retain` | Works | Deletes large ranges on DEFAULT partition can be heavy |
| `server_metrics_1h` never populated; `Agg1hRetentionDays` unused | P1 | Table in migration; worker never INSERTs 1h; `main.go` passes only raw+5m days | 30d charts / long history incomplete | Docs/DoD claim “retention/aggregation done” overstates |
| Orphan cleanup migration | — | `003_orphan_inventory_cleanup.sql` | Heals drift | Good |
| `api_tokens` in DATA_MODEL only | P4 | `docs/DATA_MODEL.md` | Not implemented | Doc drift |

---

## 7. Monitoring & metrics + stale handling

| Finding | Sev | Evidence | Current behavior | Risk |
|---------|-----|----------|------------------|------|
| Offline detection worker | — | `markOffline` every 15s vs `AGENT_OFFLINE_AFTER_SECONDS` | Sets status/health offline | Good |
| UI freshness labels | — | `format.ts` `freshnessLabel` | Fresh / Delayed / Stale / Offline | Good |
| Alert eval skips stale host samples | — | `evalHostThreshold` `time.Since(ts) > duration*2` | Avoids firing on ancient points | Partial (see §9) |
| Self-metrics endpoint | — | `/overview/self`, Metrics page | Real counts / ingest volume | Good |
| Latest metrics via LATERAL join | — | `handlers_fleet.go` | Null metrics when none | Empty ≠ fake |

---

## 8. Historical metrics & charts

| Finding | Sev | Evidence | Current behavior | Risk |
|---------|-----|----------|------------------|------|
| History API reads **raw only**, LIMIT 5000 | P1 | `handleServerMetricsHistory` | Ranges include `30d` but raw retention default **7 days** | UI offers 30d that cannot be accurate; dense 7d may truncate |
| No switch to 5m/1h aggregates for long ranges | P1 | Same handler; unused `server_metrics_5m` for reads | Charts never use downsampled tables | Performance + retention mismatch |
| ECharts on server detail | — | `servers/[id]/page.tsx` | Real series or empty message | Good empty handling |
| Fleet Metrics page is latest samples, not fleet charts | P3 | `metrics/page.tsx` | Links to server detail for history | Feature gap vs UI.md trends priority |

---

## 9. Alerting engine

| Finding | Sev | Evidence | Current behavior | Risk |
|---------|-----|----------|------------------|------|
| Default rules seeded | — | `ensureDefaultRules` | CPU/RAM/disk/offline/unhealthy | Good |
| **`duration_seconds` not truly windowed** | P1 | Comment in `evalHostThreshold`: “Approximate… full windowing comes later”; only latest sample | Alerts UI shows “for Ns” but engine does not require sustained breach | False positives / false trust |
| **`cooldown_seconds` loaded but unused** | P2 | Selected in `evaluateAlerts`; never applied | Alert spam possible on flap | Noise |
| Scope (`scope_type` / `scope_ids`) unused in eval | P3 | Schema + backup include scope; eval is fleet-wide | Cannot scope rules | Feature gap |
| Ack/resolve/silence implemented | — | `handlers_extra.go` | Works for any authenticated user | See AuthZ |
| In-app notifications derived, dismissible | — | `004_notification_dismissals.sql`, `handlers_ops.go` | No external Slack/Discord yet | Matches local-first; doc if claimed otherwise |

---

## 10. Events / audit log

| Finding | Sev | Evidence | Current behavior | Risk |
|---------|-----|----------|------------------|------|
| Infrastructure events on enroll, offline, alerts, actions | — | Various handlers + worker | Timeline page `/events` | Good |
| Audit log admin-only | — | `GET /audit-logs` `requireRole("admin")` | Login failures audited | Good |
| Audit write best-effort (`_ = Exec`) | P3 | `auth.Audit` | Silent audit drop on DB error | Compliance gap |
| Events page soft-fails empty JSON | P3 | `events/page.tsx` no strong error path | Failed fetch → empty list | Misleading “quiet” fleet |

---

## 11. Frontend UX / design system gaps

| Finding | Sev | Evidence | Current behavior | Risk |
|---------|-----|----------|------------------|------|
| Functional dark-first shell + tokens | — | `layout`, CSS variables, theme sync | Usable control center | Mostly meets DoD “mostly” |
| UI.md structure not mirrored | P3 | Docs expect `components/ui`, `charts/`, `i18n/`; actual flat `components/` + `lib/` | Works but no shared chart/i18n layer | Inconsistency / harder polish |
| Sidebar lacks live alert/offline counts | P3 | `sidebar.tsx` static nav; UI.md promises counts | Operators must open pages | UX gap |
| Topology is inventory-ish, not rich map | P3 | `/topology`; DoD notes “mostly” | Acceptable MVP | Product gap |
| Dashboard customization limited | P3 | layouts table exists; free-form grid incomplete | Show/hide-ish only | DoD already notes |
| No `dangerouslySetInnerHTML` | — | web grep | Logs as text | Good XSS posture |

---

## 12. Performance risks

| Finding | Sev | Evidence | Current behavior | Risk |
|---------|-----|----------|------------------|------|
| Per-sample INSERT in metrics ingest loop | P2 | `handlers_ingest.go` host loop `Exec` each point | Fine at small N; costly at 100 hosts × 10s | Ingest latency / DB load |
| History LIMIT 5000 without aggregation | P1 | History handler | Large ranges return incomplete or heavy payloads | Chart gaps / slow detail |
| DEFAULT partition retention DELETE | P2 | `retain` | Table scans grow with fleet | Ops pain |
| WS slow-client drop | P3 | `hub.go` non-blocking send | Silent drop | Missed UI refresh (polls compensate) |
| N+1 style list+lateral latest metrics | P3 | `handleListServers` | OK for tens of servers | Watch at hundreds |

---

## 13. Reliability / failure handling

| Finding | Sev | Evidence | Current behavior | Risk |
|---------|-----|----------|------------------|------|
| Auth probe distinguishes unauthenticated vs DB failure | — | `ErrUnauthenticated` vs `auth_unavailable` | Shell avoids false logout | Good |
| Container action timeout → 202 accepted | — | `handlers_actions.go` | Documented residual | Operator may think action finished |
| Healthz DB ping | — | `handleHealthz` | Compose/CI friendly | Good |
| Agent buffers only in-loop; failed POST logged | P2 | `main.go` metrics error to stderr | No durable retry queue | Gaps in history on API outage |
| Failure-path unit tests exist (partial) | P2 | `failure_paths_test.go`, CSRF/security tests | No browser E2E | Regressions in auth cookie path possible |

---

## 14. Security (threat notes + SECURITY.md accuracy)

### Verified accurate (SECURITY.md / SECURITY_REVIEW)

- Agents dial out; Docker socket not in API/web Compose services.  
- Dashboard does not get Docker socket or agent secrets over normal APIs (env reveal gated).  
- Password hashing Argon2id; sessions hashed at rest.  
- Management mutations confirm + audit; parameterized SQL.  
- CSRF present; security headers + HSTS when `COOKIE_SECURE`.  
- Enrollment / login rate limits; agent body size caps.

### Inaccurate or incomplete vs docs

| Claim | Reality | Sev |
|-------|---------|-----|
| WSS / mTLS agent transport (`ARCHITECTURE.md`) | HTTPS + bearer secret | P3 |
| Authenticated WebSocket (`API.md`) | Origin-only WS | P1 |
| Secrets encrypted at rest (`SECURITY.md` principle 6) | `secrets` unused; `SESSION_SECRET` unused for crypto | P2 |
| Viewer read-only (`SECURITY.md`) | Viewer can ack/silence/resolve alerts and create servers | P1 |
| Authz on mutating routes **done** (`SECURITY_REVIEW.md`) | Incomplete for alerts + server create | P1 |
| OpenAPI shared contracts | `packages/shared` empty | P3 |
| Agent update verifies SHA256 | Optional if SHA256SUMS missing | P1 |

### Additional threats

- Compromised CDN without SHA256SUMS → agent binary swap.  
- Exposed API on WAN without TLS/`COOKIE_SECURE` → session theft.  
- Default Compose Postgres password if `.env` not rotated.  
- In-memory rate limits bypassed via distributed IPs / restart.

---

## 15. Testing & CI gaps

| Finding | Sev | Evidence | Current behavior | Risk |
|---------|-----|----------|------------------|------|
| API: unit + failure-path tests | P2 | `apps/api/.../*_test.go` | No DB integration / agent E2E | Gaps |
| Agent: **no tests** | P1 | CI `go test` empty suite | False green | High |
| Web: lint + build only; `pnpm audit \|\| true` | P1 | `.github/workflows/ci.yml` | No Vitest/Playwright | Auth/CSRF regressions possible |
| Compose config + image build in CI | — | `compose` job | Good smoke for packaging | Does not boot full stack + migrate assert |
| No browser smoke in CI | P2 | DoD open item | Manual only | Release risk |

---

## 16. Deployment / backup / docs accuracy

| Finding | Sev | Evidence | Current behavior | Risk |
|---------|-----|----------|------------------|------|
| Compose db/api/web (+ optional tunnel) documented | — | `deploy/docker-compose.yml`, `DEPLOYMENT.md` | Matches | Good |
| Backup API exports settings/rules; full PG dump recommended | P2 | Settings UI + `DEPLOYMENT.md`; restore does not recreate servers | Correct caution | Operators may think Settings backup = full DR |
| Migrations on API startup | — | `db.Migrate` | Good | Need backup before upgrade still |
| DoD marks aggregation/history “done” | P1 | `DEFINITION_OF_DONE.md` vs unused 1h + raw-only history | Overclaim | Premature “production complete” |
| RELEASE / agent CDN / tunnel docs exist | — | `REMOTE_AGENTS.md`, `RELEASE.md` | Operational path clear | Good |
| Dev default secrets in examples | P2 | `.env.example`, Compose `fleetdeck_dev_change_me` | Expected for local | Must rotate for any shared deploy (documented) |

---

## Fake / demo data checklist

| Pattern | Result |
|---------|--------|
| `Math.random` metrics | Not found in apps source |
| `faker` / mock metric services | Not found (test `fakeRows` only) |
| Hardcoded CPU/RAM demo constants in UI | Not found; null → `—` / empty copy |
| PlaceholderPage fake charts | Explicitly empty; unused by routes |

**Verdict:** Production UI/API paths use agent-reported and DB-derived data. Primary integrity risks are **incomplete history aggregation** and **alert duration approximation**, not fabricated numbers.

---

## Top P0 / P1 (for Improvement Plan)

No classic P0 “dashboard invents metrics” or open RCE via Docker shell injection was found.

**P1 (treat as production blockers before shared exposure):**

1. Unauthenticated `/api/v1/realtime` WebSocket.  
2. AuthZ gaps: viewer alert mutations + server create; SECURITY_REVIEW overclaim.  
3. Metrics history does not use aggregates; 1h pipeline dead; 30d UI vs 7d raw.  
4. Alert `duration_seconds` / cooldown not enforced as documented.  
5. Agent update allows missing SHA256SUMS.  
6. Agent has no tests; web CI has no tests; advisory-only npm audit.

---

## Appendix — key evidence paths

- Compose: `deploy/docker-compose.yml`  
- Schema: `apps/api/internal/db/migrations/001_init.sql` … `004_*.sql`  
- Auth/CSRF: `apps/api/internal/auth/`, `apps/api/internal/api/csrf.go`, `security.go`  
- Ingest/actions: `handlers_ingest.go`, `handlers_actions.go`, `handlers_fleet.go`  
- Worker: `apps/api/internal/worker/runner.go`  
- Agent: `apps/agent/cmd/fleetdeck-agent/`, `internal/collect/`  
- Web client: `apps/web/src/lib/api.ts`, `format.ts`, `realtime.ts`  
- CI: `.github/workflows/ci.yml`  
- Security docs: `docs/SECURITY.md`, `docs/SECURITY_REVIEW.md`
