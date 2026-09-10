#!/usr/bin/env bash
# FleetDeck non-root / shared-host installer (no sudo).
# Equivalent to:
#   curl -fsSL https://cdn.example.com/fleetdeck/install.sh | bash -s -- --user --token 'TOKEN'
set -euo pipefail
CDN_BASE="${FLEETDECK_CDN:-https://cdn.example.com/fleetdeck}"
curl -fsSL "${CDN_BASE}/install.sh" | bash -s -- --user "$@"
