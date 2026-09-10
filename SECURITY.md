# Security Policy

## Supported versions

FleetDeck is pre-1.0 (`*-dev` releases). Security fixes land on `master` and are called out in [docs/RELEASE_NOTES.md](docs/RELEASE_NOTES.md).

| Version | Supported |
|---------|-----------|
| `0.4.x-dev` (latest `master`) | Yes |
| Older `0.4.x-dev` tags | Best-effort — upgrade when possible |
| Pre-0.4 | No |

## Reporting a vulnerability

**Please do not open a public GitHub issue for security vulnerabilities.**

Report privately via one of:

1. **GitHub Security Advisories** — [Report a vulnerability](https://github.com/fleames/fleetdeck/security/advisories/new) on this repository (preferred)
2. **Email** — open a private advisory request through GitHub if email contact is unavailable on the profile

Include:

- Affected component (API, agent, web, install/upgrade scripts, Compose)
- Version / commit if known
- Steps to reproduce or a proof of concept
- Impact assessment (auth bypass, secret exposure, RCE, etc.)

We aim to acknowledge reports within **7 days** and to share a remediation plan or fix timeline once confirmed.

## Scope notes

In scope examples:

- Authn/authz bypass on the dashboard or agent protocol
- Exposure of session secrets, agent credentials, or envelope-encrypted rows
- Unsafe handling of enrollment tokens or CDN update verification
- Path traversal / command injection in install or update helpers

Out of scope examples:

- Denial of service against a self-hosted instance without a clear protocol bug
- Issues that require physical access or an already-compromised host agent
- Misconfiguration of Cloudflare Tunnel, R2, or reverse proxies outside this repo

## Hardening references

- Threat model and controls: [docs/SECURITY.md](docs/SECURITY.md)
- Residual review notes: [docs/SECURITY_REVIEW.md](docs/SECURITY_REVIEW.md)
