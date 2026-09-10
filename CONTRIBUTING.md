# Contributing to FleetDeck

Thanks for helping improve FleetDeck. This project is a **self-hosted** fleet monitoring stack (Go API + Next.js dashboard + Go agent). Small, focused changes are easiest to review.

## Before you start

1. Read [README.md](README.md) and [docs/README.md](docs/README.md).
2. Follow the [Code of Conduct](CODE_OF_CONDUCT.md).
3. For security issues, use [SECURITY.md](SECURITY.md) — do not file a public bug.

## Development setup

```bash
cp .env.example .env
# Set SESSION_SECRET (openssl rand -base64 48)

# Full stack
docker compose -f deploy/docker-compose.yml up -d --build

# Or split processes — see docs/DEVELOPMENT.md
docker compose -f deploy/docker-compose.yml up -d db
cd apps/api && go run ./cmd/fleetdeck-api
cd apps/web && pnpm install && pnpm dev
```

Prerequisites: **Go 1.26+** (API), **Go 1.25+** (agent), **Node 24+ / pnpm 10+**, Docker.

## Making changes

- Match existing style in the package you touch (`apps/api`, `apps/agent`, `apps/web`).
- Prefer real metrics and inventory paths — no fake production data generators.
- Do not commit `.env`, credentials, PEM/keys, `bin/`, or `dist/` artifacts.
- Keep PRs focused: one concern per PR when practical.

### Checks to run locally

```bash
# API
cd apps/api && go test ./... -count=1 && go build -o /tmp/fleetdeck-api ./cmd/fleetdeck-api

# Agent
cd apps/agent && go test ./... -count=1 && go build -o /tmp/fleetdeck-agent ./cmd/fleetdeck-agent

# Web
cd apps/web && pnpm lint && pnpm test && pnpm build

# Compose config
docker compose -f deploy/docker-compose.yml config
```

CI (`.github/workflows/ci.yml`) runs these plus image builds and a Playwright smoke test.

## Pull requests

1. Fork (or branch from `master`) and open a PR against `master`.
2. Fill out the PR template: what changed, why, and how you tested.
3. Link related issues.
4. Expect review on security-sensitive paths (auth, agent enroll/update, secrets).

## Issues

- Use [bug](.github/ISSUE_TEMPLATE/bug_report.yml) or [feature](.github/ISSUE_TEMPLATE/feature_request.yml) templates.
- Include FleetDeck version (`0.4.x-dev`), OS, and whether you use Compose / Tunnel / CDN.

## Docs

Doc map lives in [docs/README.md](docs/README.md). Prefer updating the relevant guide when behavior changes — especially [AGENT.md](docs/AGENT.md), [DEPLOYMENT.md](docs/DEPLOYMENT.md), and [RELEASE_NOTES.md](docs/RELEASE_NOTES.md).

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
