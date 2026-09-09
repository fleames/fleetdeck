# Definition of Done — status

Honest tracking against the product goal. **Not complete** until every row is verified with evidence.

| Requirement | Status | Evidence |
|-------------|--------|----------|
| Architecture + audit docs | done | `docs/AUDIT.md`, `ARCHITECTURE.md`, … |
| Dashboard + Monitoring API + Agent + Local DB | done | `apps/web`, `apps/api`, `apps/agent`, Postgres |
| Secure agent-based collection | done | Enrollment, hashed secrets, dial-out agent |
| Real metrics (no fake production data) | done | gopsutil + Docker Engine API collectors |
| Historical retention / aggregation | done | Workers + raw/5m retention settings |
| Alerts / events | done | Rules, instances, events UI |
| Polished dark-first UX (+ light theme) | mostly | Design tokens, theme toggle, live logs page; light theme applied via settings |
| Auth / secrets security | mostly | Argon2id, CSRF, rate limits, roles, HSTS-when-secure; TLS still via reverse proxy |
| Export / backup / notifications / self-metrics | done | Settings + API ops handlers |
| Container actions + env mask/reveal | done | Confirm + audit; agent 0.4.0-dev |
| Server compare / capacity notes | done | `/compare`, `POST /servers/compare` |
| Topology map / dashboard customization | mostly | `/topology` inventory view; widget show/hide (not free-form drag grid) |
| Multi-user roles UX | done | Admin Users page + API role enforcement |
| Comprehensive tests + failure paths | mostly | Unit/failure-path tests + GitHub Actions CI; no full browser E2E suite |
| Docker Compose deployment verified | done | Docker Desktop started; `db`/`api`/`web` up; `/healthz` OK, web 200; agent 0.4.0-dev online |
| Full documentation | mostly | Deploy/TLS/agent/release/security/API/troubleshoot/DoD |
| Production release package | mostly | Compose images + agent multi-arch + SHA256SUMS; GPG signing optional/manual |

**Open before calling the goal complete**

1. Re-verify Compose stack health with Docker Desktop running (`up -d` + `/healthz` + web 200).
2. Optional but expected for “production”: GPG-signed `SHA256SUMS` in a tagged release.
3. Prefer a minimal browser smoke (login → dashboard → servers) if automation is added.

**Verdict:** Goal remains **active**. Core product is largely shipped; finish the open verification/signing items above before marking complete.
