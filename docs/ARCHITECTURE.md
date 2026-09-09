# Architecture

## Mission restatement (verified against)

FleetDeck aggregates **real** infrastructure state from enrolled servers into one local application: online/offline, CPU/RAM/disk/network/load/uptime/temps, OS/kernel/hardware, Docker daemon + containers/images/volumes/networks/compose, logs, alerts, events, and historical trends — with agent-based security, no fake production data, and premium UX.

## High-level components

```text
                 ┌──────────────────────────┐
                 │   Dashboard (apps/web)   │
                 │  Next.js · design system │
                 └────────────┬─────────────┘
                              │ HTTPS + WS/SSE
                              ▼
                 ┌──────────────────────────┐
                 │ Monitoring API (apps/api)│
                 │ auth · ingest · alerts   │
                 │ registry · aggregation   │
                 └──────┬─────────┬─────────┘
                        │         │
              ┌─────────┘         └─────────┐
              ▼                             ▼
     ┌────────────────┐            ┌────────────────┐
     │ Local Database │            │ Agent gateway  │
     │ PG + metrics   │            │ WSS / mTLS     │
     └────────────────┘            └───────┬────────┘
                                           │ agents dial out
                     ┌─────────────────────┼─────────────────────┐
                     ▼                     ▼                     ▼
               ┌──────────┐          ┌──────────┐          ┌──────────┐
               │ Agent 1  │          │ Agent 2  │          │ Agent N  │
               │ collectors│         │          │          │          │
               └────┬─────┘          └────┬─────┘          └────┬─────┘
                    │                     │                     │
                 host+Docker           host+Docker           host+Docker
```

## Stack proposal (Phase 2+)

| Layer | Choice | Rationale |
|-------|--------|-----------|
| Language (API + Agent) | **Go** | Small static agent binaries; strong concurrency for ingest; single ops story |
| Dashboard | **Next.js (App Router) + React + TypeScript** | App-quality UI, SSR/static shell, excellent DX |
| Styling | **Tailwind + design tokens** (custom, not generic admin kit) | Dark-first premium control; consistent spacing/type |
| Charts | **Apache ECharts** (or uPlot for ultra-dense sparklines) | Interactive hover, dual themes |
| Relational DB | **PostgreSQL 16** | Servers, users, alerts, audit, Docker metadata |
| Time-series | **PostgreSQL partitioned tables** (+ optional Timescale later) | Local-first with one DB in Compose; retention jobs in API |
| Cache / realtime | In-process hub first; **Redis optional** later | Avoid mandatory extra moving parts for single-node |
| Shared contracts | **OpenAPI + JSON Schema** generated to TS/Go | Strict typing at boundaries; no `any` drift |
| Packaging | **pnpm/npm workspaces** for web; Go modules for api/agent; root Compose | Clear separation |
| Deploy | `docker compose up -d` for API+DB+web; agent as binary/package | Matches PRD |

### Alternatives considered

| Alternative | Why not default |
|-------------|-----------------|
| Evolve PulseOps in place | Product rewrite; threat/log core conflicts with fleet IA |
| TypeScript-only API (Hono) | Fine for web teams; Go preferred for agent footprint + ingest |
| SQLite-only | Risky write amplification at 100 hosts × high-frequency metrics |
| Mount Docker socket in API container | Violates security model; agent owns local Docker access |
| Prometheus remote_write as primary | Powerful but heavier UX/ops; can add exporters later |

## Logical layers

### Dashboard

```text
UI (pages/widgets)
  → API client + realtime store
  → Presentation hooks (freshness, health score explainers)
```

No DB/Docker/auth business logic in components.

### Monitoring API

```text
HTTP/WS handlers
  → Services (servers, metrics, docker, alerts, auth, audit)
  → Repositories
  → PostgreSQL
```

Separate: monitoring (read), diagnostics, management (mutating, confirmed).

### Agent

```text
Collectors (system / docker / meta)
  → Normalizer (canonical metric schema)
  → Buffer + backpressure
  → Transport (authenticated WSS push)
  → Optional command channel (explicit management only)
```

## Data flow

1. Operator creates **enrollment token** in Dashboard.
2. Agent installed on host with token + API URL; dials out.
3. API exchanges token for long-lived **agent credentials** (rotatable).
4. Agent sends heartbeats + batched high-frequency metrics; low-frequency inventory on slower cadence.
5. API validates, persists raw window, updates last-seen, evaluates alerts, fans out realtime events.
6. Retention worker downsamples and deletes expired raw points.
7. Dashboard reads aggregated APIs; never talks to agents or Docker sockets directly.

## Failure isolation

- Unreachable agent → server **Offline**; last-known + freshness shown; other servers unaffected.
- Docker unavailable on host → system metrics continue; Docker section shows explicit failure.
- Slow agent → buffered ingest; UI shows delayed/stale, never silently “current”.
- Corrupt payload → reject + audit/diagnostic event; no partial trust.

## Performance strategy

- Async ingest; never block UI on one host
- Batch metric writes; N+1-free list endpoints with embeddings or join views
- Pagination + virtualized lists for large fleets
- Cached dashboard snapshot endpoint for “is everything okay?” in ~1s when warm
- Single browser realtime connection (multiplexed topics)

## Phase alignment

| Phase | Focus |
|-------|--------|
| 1 | This architecture (current) |
| 2 | Foundation: repo, DB, auth, design system shell |
| 3 | Agent + enrollment + real metrics |
| 4 | Dashboard overview + server detail + realtime |
| 5 | Docker deep surfaces + logs |
| 6 | Alerts + events + notifications architecture |
| 7 | Search, customization, comparisons, topology |
| 8 | Hardening |
| 9 | Documentation completion |
| 10 | Release packages |

## Non-goals (initial release)

- Mandatory cloud SaaS, multi-region check grid, Stripe billing
- Replacing Prometheus/Grafana for arbitrary custom metrics (can integrate later)
- Unattended destructive Docker operations
