# UI architecture

## Product feel

Premium infrastructure command center: **Grafana + Portainer + modern SaaS + native desktop density**, cohesive and local-first — not a generic Bootstrap admin template.

Dark mode is primary; light mode supported. Color communicates state but **never alone** (always include text: Healthy / Critical / Offline).

## Information priority

1. Infrastructure health  
2. Critical alerts  
3. Offline servers  
4. Unhealthy containers  
5. Resource pressure  
6. Trends  
7. Details  

Opening the app must answer: **“Is everything okay?”** in under ~5 seconds.

## App structure

```text
apps/web
  src/
    app/                 # routes (App Router)
    components/
      ui/                # primitives (button, input, skeleton…)
      charts/            # shared chart wrappers + theme
      servers/           # cards, detail tabs
      docker/            # containers, compose, topology
      alerts/
      layout/            # shell, sidebar, topbar
    lib/
      api/               # typed client
      realtime/          # single WS store
      freshness/         # Fresh|Delayed|Stale|Offline helpers
      i18n/              # message catalogs (English first)
    styles/
      tokens.css         # design tokens
```

## Navigation

```text
OVERVIEW
  Dashboard
  Servers
  Docker
  Containers
  Compose
  Images
  Volumes
  Networks

OBSERVABILITY
  Metrics
  Alerts
  Events
  Logs

SYSTEM
  Settings
  Agents
  Users
  Audit Log
  About
```

Sidebar shows live counts: Alerts, Unhealthy, Offline.

## Key screens (Phase mapping)

| Screen | Phase |
|--------|-------|
| Shell + empty states + auth | 2 |
| Dashboard overview + server cards | 4 |
| Server detail tabs | 4–5 |
| Docker summary + container explorer/detail/logs | 5 |
| Compose / images / volumes / networks | 5 |
| Alert center + event timeline | 6 |
| Ctrl+K search, customization, compare, topology | 7 |

## Visual system

### Tokens

CSS variables for: background layers, borders, text hierarchy, status colors (info/warn/critical/ok/offline/maintenance), chart series, spacing scale, radii (restrained), motion durations.

Avoid: purple-gradient AI clichés, excessive glassmorphism, huge empty hero voids, pill spam, multi-shadow stacks. Prefer technical clarity and density with breathing room where it aids hierarchy.

### Components

- Server cards with resource bars + sparklines + freshness
- Status dots **with labels**
- Skeletons for loading; cached previous values where safe
- Empty states with one clear CTA (e.g. Add Server)
- Error states with human message + expandable diagnostics
- Confirm dialogs for destructive/management actions

### Charts

- Interactive hover with exact values
- Theme-aware palettes (readable in dark and light)
- Time range control: 15m / 1h / 6h / 24h / 7d / 30d / custom
- No fake series; show “Metrics unavailable” + last success when empty

## Realtime UX

- One multiplexed connection
- Smooth metric updates without layout jitter
- Connection indicator in shell
- Offline server cards remain usable with last-known + historical navigation

## Global search

`Ctrl+K` / `Cmd+K` command palette: servers, containers, images, compose, volumes, networks, alerts, events. Instant client filter over prefetched indexes + server search fallback.

## Accessibility

- Keyboard navigation and visible focus
- Semantic landmarks
- ARIA for live regions (alert toasts) without aggression
- Contrast meeting WCAG AA where feasible in both themes
- `prefers-reduced-motion` respected

## Responsive

Desktop primary. Tablet/mobile: collapse sidebar, stack cards, simplify charts, prioritize health + alerts.

## State management

- Server state via typed API hooks (React Query or equivalent)  
- Realtime overlay store merges WS deltas  
- No business logic duplication of alert evaluation in the browser  

## i18n readiness

User-visible strings via message catalogs; English default. No hardcoded copy inside API error mapping beyond shared codes.
