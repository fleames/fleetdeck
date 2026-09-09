# Phase 1 verification gate

Per the project plan: **stop and verify architecture before massive implementation.**

## Deliverables checklist

| Deliverable | Path | Status |
|-------------|------|--------|
| Repository audit | [AUDIT.md](AUDIT.md) | Draft complete |
| Architecture proposal | [ARCHITECTURE.md](ARCHITECTURE.md) | Draft complete |
| Data model | [DATA_MODEL.md](DATA_MODEL.md) | Draft complete |
| Security model | [SECURITY.md](SECURITY.md) | Draft complete |
| API design | [API.md](API.md) | Draft complete |
| Agent protocol | [AGENT.md](AGENT.md) | Draft complete |
| UI architecture | [UI.md](UI.md) | Draft complete |

## Decisions to confirm before Phase 2

Please confirm or adjust:

1. **Greenfield `fleetdeck`** (not forking PulseOps / PulseGuard / Sentinel) — recommended.
2. **Stack:** Go API + Go agent + Next.js web + PostgreSQL (+ Compose).
3. **Product name / folder:** `fleetdeck` under `Projects\` (rename if you prefer).
4. **Realtime:** WebSocket multiplex first (SSE acceptable for logs).
5. **Time-series:** PostgreSQL partitioned metrics first (Timescale optional later).
6. **Management actions:** deferred behind feature flag until after read-only monitoring is solid — still design confirmation dialogs now.

## Explicitly out of Phase 1

- Application runtime code, DB migrations, UI implementation
- Agent binaries
- Production deployment hardening

## Approval → Phase 2

When approved, Phase 2 implements:

- Monorepo scaffold (`apps/api`, `apps/web`, `apps/agent`, `packages/shared`)
- PostgreSQL schema migrations for core tables
- Auth bootstrap + session security baselines
- Base UI shell + design tokens (dark-first)
- `.env.example`, Compose skeleton, initial tests/lint/typecheck
- Verify build + tests before Phase 3

## How to respond

Reply with approval, or list change requests (name, stack, DB, or “evolve PulseOps instead”).
