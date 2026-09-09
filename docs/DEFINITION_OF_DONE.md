# Definition of Done — status

Honest tracking against the product goal. Remaining gaps are **intentionally accepted** local-first limits or **Future** (P4) work — see [PRODUCTION_READINESS.md](./PRODUCTION_READINESS.md).

| Requirement | Status | Evidence |
|-------------|--------|----------|
| Architecture + audit docs | done | `docs/AUDIT.md`, `ARCHITECTURE.md`, `PRODUCTION_AUDIT.md`, `PRODUCTION_READINESS.md` |
| Dashboard + Monitoring API + Agent + Local DB | done | `apps/web`, `apps/api`, `apps/agent`, Postgres |
| Secure agent-based collection | done | Enrollment, hashed secrets, dial-out agent; mandatory update SHA256 |
| Real metrics (no fake production data) | done | gopsutil + Docker Engine API collectors |
| Historical retention / aggregation | done | Worker raw→5m→1h; monthly partition create/DROP + DELETE fallback; history API by range |
| Alerts / events | done | Rules, instances, events UI; duration/cooldown; `scope_type`/`scope_ids`; Discord/Slack-compatible webhook + Settings |
| Polished dark-first UX (+ light theme) | mostly | Design tokens, theme toggle, page-state, topology polish, a11y basics; no free-form drag grid (**Future**) |
| Auth / secrets security | mostly | Argon2id, CSRF, DB-backed login/enroll rate limits, roles, WS session auth, HSTS-when-secure; TLS via reverse proxy; envelope Put/Delete + Settings signing secret; agent creds 0600 + threat model |
| Export / backup / notifications / self-metrics | done | Settings + API ops; in-app notifications + webhook channels; expanded `/healthz` + self-metrics |
| Container actions + env mask/reveal | done | Confirm + audit; agent allowlisted actions |
| Server compare / capacity notes | done | `/compare`, `POST /servers/compare` |
| Topology map / dashboard customization | mostly | `/topology` live inventory map + summary tiles; widget show/hide (drag grid **Future**) |
| Multi-user roles UX | done | Admin Users page + API role enforcement (viewer read-only mutations) |
| Comprehensive tests + failure paths | mostly | API/agent/web unit tests + Playwright smoke in CI; no full DB integration suite |
| Docker Compose deployment verified | done | `db`/`api`/`web` up; expanded `/healthz` 200; web 200; CI e2e compose smoke |
| Full documentation | mostly | Deploy/TLS/agent/release/security/API/troubleshoot/DoD/MONITORING/DATABASE/readiness |
| Production release package | mostly | Compose images + agent multi-arch + SHA256SUMS; GPG signing **Future**/optional |

**Accepted limits (not open blockers)**

1. Control-plane **SPOF** (single API + reachable public URL) — local-first single-node design.
2. Metrics **DEFAULT** partition residual DELETE for leftover/historical rows.

**Future (non-blocking)**

- Optional GPG-signed `SHA256SUMS`, OpenAPI/`packages/shared`, free-form drag grid, alert email (SMTP).

**Verdict:** Goal **COMPLETE**. Readiness: **READY WITH KNOWN LIMITATIONS** — suitable for local-first / private fleet behind TLS; not bare READY / HA.
