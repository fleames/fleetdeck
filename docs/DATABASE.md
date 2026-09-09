# Database

Postgres 16 (Compose) is the single store for registry, inventory, metrics, alerts, audit, and sessions.

## What is stored where

| Data | Storage | At-rest notes |
|------|---------|---------------|
| User passwords | `users.password_hash` | Argon2id |
| Session tokens | `sessions.token_hash` | SHA-256 of opaque cookie; cookie not encrypted with `SESSION_SECRET` |
| Agent secrets | `agent_credentials.secret_hash` | Argon2id; agent keeps plaintext in `credentials.json` mode 0600 |
| Enrollment tokens | `enrollment_tokens.token_hash` | Hashed |
| App secrets table | `secrets` (ciphertext, nonce, key_version) | AES-256-GCM via `SESSION_SECRET`-derived key (`apps/api/internal/secrets`). **Ready for Put/Get; no product feature writes rows yet** |
| Metrics | `server_metrics_raw` / `_5m` / `_1h`, `container_metrics_raw` | Not encrypted; protect via DB access + volume permissions |

## `SESSION_SECRET`

Required (≥ 32 chars). Used to **derive the envelope encryption key** for the `secrets` table. It is **not** used to sign session cookies (sessions are opaque + DB-hashed). Rotate carefully: existing `secrets` ciphertext becomes undecryptable if the secret changes without re-encrypt.

## Migrations

Embedded SQL under `apps/api/internal/db/migrations/`, applied on API start via `schema_migrations`.

## Backups

- Volume / `pg_dump` of the Postgres data directory is the restore path for metrics and inventory.
- Export endpoints and Settings JSON are config/convenience — not a full DR strategy.

See also: [DATA_MODEL.md](./DATA_MODEL.md), [MONITORING.md](./MONITORING.md), [DEPLOYMENT.md](./DEPLOYMENT.md).
