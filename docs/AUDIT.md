# Repository audit

**Date:** 2026-09-08  
**Scope:** Local workspace scan for an existing codebase matching this product goal.  
**Decision:** **Greenfield** (`fleetdeck`). Do not rewrite or fork existing products.

No repository was attached to the goal. Nearby related codebases were inspected so we prefer incremental engineering where it truly fits.

---

## Candidates inspected

### 1. Related traffic / threat observability monorepo — closest, still wrong product core

**What it is:** Self-hosted **traffic + threat observability** (log ingest, security center, SSL, reports) with **secondary** SSH-polled system/Docker pages.

| Area | Finding |
|------|---------|
| Structure | `backend/` (Go Fiber) + `frontend/` (Next.js 16) + Docker Compose |
| Framework | Go 1.25, Fiber, WebSocket hub, DuckDB analytics, SQLite config |
| Frontend | Next.js App Router, Tailwind, shadcn, ECharts, dark-first |
| Auth | bcrypt + HMAC JWT; soft-gate until first admin; API keys |
| Docker | via **SSH** (`docker ps` / `docker stats`) — no agent |
| Metrics | via SSH; DuckDB `system_metrics`; limited history API |
| Alerts | Real rule engine + channels (Discord/Slack/etc.) |
| Tests / CI | Partial; not a full fleet-agent E2E story |
| Security model | Avoids Docker socket in the UI; relies on SSH credentials to hosts |

#### Keep (ideas / patterns worth learning from)

- Local-first Compose deployment
- Dark-first dense dashboard UX + ECharts
- DuckDB (or similar) for analytics + SQLite/PG for config split
- Soft-first-admin bootstrap then harden writes
- Alert engine as a first-class service, not UI-only storage
- Single shared WebSocket fan-out (not per-widget connections)

#### Improve (if we were evolving that codebase)

- Replace SSH polling with outbound agent protocol
- Multi-server registry, enrollment tokens, agent health
- Full Docker inventory (images/volumes/networks/compose/logs)
- Retention/downsampling policy as product requirement
- Stronger RBAC, CSRF, encrypted secrets at rest, audit completeness

#### Replace (fundamentally unsuitable for this goal)

- **Product center of gravity:** HTTP threat pipeline as the primary loop
- **Collection model:** central SSH into hosts (credentials sprawl; not agent-based)
- **Domain model:** domains/traffic/threat events vs servers/agents/containers/stacks
- Navigation and IA built around Security/Live Stream/Traffic, not fleet health

#### Missing vs FleetDeck PRD

- Dedicated agent + enrollment
- Infrastructure health score, topology, capacity planning
- Compose graphs, image/volume/network management UX
- Container log viewer with safe rendering
- Dashboard customization, comparisons, Ctrl+K command palette depth
- Documented retention strategy and self-observability
- Management actions with confirmation (read-only default)

**Verdict:** Reusing that product as the base would force a rewrite under an existing name. **Do not fork.** Borrow patterns selectively.

---

### 2. Related uptime / status-page SaaS monorepo

SaaS-style **uptime / SSL / status pages / billing** (Next.js + Hono + Prisma + Redis + Stripe). Multi-tenant cloud product shape.

**Verdict:** Unsuitable foundation for local-first agent/Docker fleet monitoring.

---

### 3. Related SOC / access-log dashboard

Python Flask **SOC dashboard** for reverse-proxy JSON access logs + SSH brute-force telemetry.

**Verdict:** Different domain (attack telemetry). Not a fleet/Docker command center.

---

### 4. Other local projects

No existing monitoring dashboard project matching this goal among nearby workspaces inspected at the time.

---

## Summary matrix

| Requirement | Threat obs. | Uptime SaaS | SOC logs | FleetDeck (new) |
|-------------|-------------|-------------|----------|-----------------|
| Local-first | Yes | Partial | Yes | Yes |
| Agent-based | No (SSH) | No | Log agents only | Yes |
| Multi-server fleet UI | Thin | Monitors | Single host focus | Core |
| Docker deep inventory | Containers only | No | No | Core |
| Historical metrics + retention | Partial | Checks | Limited | Core |
| Premium cohesive UX | Strong start | Marketing SaaS | SOC | Target |
| Matches PRD mission | ~30% | ~15% | ~10% | 100% target |

---

## Engineering recommendation

1. **Create greenfield monorepo `fleetdeck`.**
2. **Reuse lessons**, not code dumps: Go agent + Go API option, Next.js dark UI, typed boundaries, DuckDB/PG time-series thinking, alert engine separation, WS fan-out.
3. **Phase 1 stop gate:** approve architecture docs before scaffolding runtime code (Phase 2).
