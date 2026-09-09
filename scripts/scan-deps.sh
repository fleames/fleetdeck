#!/usr/bin/env bash
# Local dependency vulnerability scans for API, agent, and web.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

echo "== API govulncheck =="
(cd "$ROOT/apps/api" && go run golang.org/x/vuln/cmd/govulncheck@latest ./...)

echo "== Agent govulncheck =="
(cd "$ROOT/apps/agent" && go run golang.org/x/vuln/cmd/govulncheck@latest ./...)

echo "== Web pnpm audit (prod) =="
(cd "$ROOT/apps/web" && pnpm audit --prod || true)

echo "Done."
