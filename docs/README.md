# FleetDeck documentation

Self-hosted fleet monitoring: Go API, Next.js dashboard, dial-out Go agents, PostgreSQL.

Start with the root [README](../README.md) for value prop, screenshots, and Docker Compose quick start. Use this index to find deeper guides.

## Newcomers

| Doc | When to read it |
|-----|-----------------|
| [DEPLOYMENT.md](DEPLOYMENT.md) | First self-host, env vars, TLS, backup, Windows autostart |
| [REMOTE_AGENTS.md](REMOTE_AGENTS.md) | Cloudflare Tunnel + CDN so remote hosts can enroll |
| [AGENT.md](AGENT.md) | Agent role, panel update/uninstall, flags, CDN install |
| [TROUBLESHOOTING.md](TROUBLESHOOTING.md) | Common failures (API down, pending enroll, agent RAM, etc.) |
| [SECURITY.md](SECURITY.md) | Threat model and controls |
| [RELEASE_NOTES.md](RELEASE_NOTES.md) | What changed in `0.4.x-dev` |

## Architecture & product

| Doc | Topic |
|-----|--------|
| [ARCHITECTURE.md](ARCHITECTURE.md) | Components, data flow, stack choices |
| [DATA_MODEL.md](DATA_MODEL.md) | Schema / entities |
| [API.md](API.md) | HTTP API surface |
| [UI.md](UI.md) | Dashboard IA and UX principles |
| [MONITORING.md](MONITORING.md) | Metrics, retention, webhooks |
| [DATABASE.md](DATABASE.md) | Postgres notes |

## Ops & release

| Doc | Topic |
|-----|--------|
| [DEVELOPMENT.md](DEVELOPMENT.md) | Split-process local workflow |
| [RELEASE.md](RELEASE.md) | Packaging and release |
| [DEFINITION_OF_DONE.md](DEFINITION_OF_DONE.md) | Production readiness checklist |
| [PRODUCTION_READINESS.md](PRODUCTION_READINESS.md) | Readiness verdict and limits |
| [SECURITY_REVIEW.md](SECURITY_REVIEW.md) | Hardening residuals |
| [AUDIT.md](AUDIT.md) | Historical greenfield decision notes |

## Screenshots

Anonymized UI captures used in the README live under [`screenshots/`](screenshots/).

## Contributing & security

- [Contributing guide](../CONTRIBUTING.md)
- [Code of Conduct](../CODE_OF_CONDUCT.md)
- [Vulnerability reporting](../SECURITY.md) (root) — distinct from this folder's threat-model [SECURITY.md](SECURITY.md)
