# Agent release packaging

Build multi-platform agent binaries with checksums:

```bash
# Linux/macOS
VERSION=0.4.0 ./scripts/release-agent.sh

# Windows PowerShell
./scripts/release-agent.ps1 -Version 0.4.0
```

Outputs under `dist/agent/<version>/`:

- `fleetdeck-agent_<version>_<os>_<arch>[.exe]`
- `SHA256SUMS`

## Signing (recommended for production)

```bash
cd dist/agent/<version>
gpg --detach-sign --armor SHA256SUMS
```

Publish the binaries, `SHA256SUMS`, and `SHA256SUMS.asc` together. Operators verify:

```bash
gpg --verify SHA256SUMS.asc SHA256SUMS
sha256sum -c SHA256SUMS
```

API and web are released as Docker images via Compose (`deploy/docker-compose.yml`). Agents are host binaries and are **not** auto-updated.
