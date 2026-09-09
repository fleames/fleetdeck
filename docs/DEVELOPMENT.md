# Development

## Prerequisites

- Go 1.26+
- Node 24+ / pnpm 10+
- Docker (PostgreSQL)

## Run locally

```bash
cp .env.example .env
docker compose -f deploy/docker-compose.yml up -d db

# API (loads ../../.env)
cd apps/api
go run ./cmd/fleetdeck-api

# Web
cd apps/web
pnpm dev

# Agent after UI enrollment token
cd apps/agent
go run ./cmd/fleetdeck-agent -api http://localhost:8080 -token <token>
```

Postgres is mapped to **localhost:5433** to avoid clashing with other local Postgres instances.

## Verify

```bash
cd apps/api && go test ./... -count=1 && go build -o ../../bin/fleetdeck-api.exe ./cmd/fleetdeck-api
cd apps/agent && go build -o ../../bin/fleetdeck-agent.exe ./cmd/fleetdeck-agent
cd apps/web && pnpm lint && pnpm build
curl http://localhost:8080/healthz
```

## Phase status

- Phase 1–5: Architecture through logs/search — done
- Phase 6: Export, theme, notifications, self-metrics, backup/restore — done
- Phase 7: Container actions, env mask/reveal, server compare — done
- Phase 8: Hardening (authz lock-down, rate limits, body limits, security review doc) — mostly done; residuals in `SECURITY_REVIEW.md`
- Remaining: topology/customization, users UX, fuller E2E, production release
