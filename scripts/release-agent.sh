#!/usr/bin/env bash
# Cross-compile FleetDeck agent release artifacts with SHA256 checksums.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="${VERSION:-0.4.0-dev}"
OUT="${ROOT}/dist/agent/${VERSION}"
mkdir -p "$OUT"

cd "${ROOT}/apps/agent"
targets=(
  "linux/amd64"
  "linux/arm64"
  "darwin/amd64"
  "darwin/arm64"
  "windows/amd64"
)

for t in "${targets[@]}"; do
  os="${t%/*}"
  arch="${t#*/}"
  ext=""
  if [[ "$os" == "windows" ]]; then ext=".exe"; fi
  name="fleetdeck-agent_${VERSION}_${os}_${arch}${ext}"
  echo "Building ${name}"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -ldflags="-s -w" -o "${OUT}/${name}" ./cmd/fleetdeck-agent
done

(
  cd "$OUT"
  if command -v sha256sum >/dev/null; then
    sha256sum fleetdeck-agent_* > SHA256SUMS
  else
    shasum -a 256 fleetdeck-agent_* > SHA256SUMS
  fi
)

echo "Artifacts in ${OUT}"
echo "Optional: gpg --detach-sign --armor SHA256SUMS"
