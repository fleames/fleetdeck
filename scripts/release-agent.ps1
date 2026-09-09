# Cross-compile FleetDeck agent release artifacts with SHA256 checksums (Windows).
# After build, publishes CDN + uploads to R2 when R2_* are set (unless -NoCdn).
param(
  [string]$Version = "0.4.0-dev",
  [switch]$NoCdn
)
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
$Out = Join-Path $Root "dist\agent\$Version"
New-Item -ItemType Directory -Force -Path $Out | Out-Null

Push-Location (Join-Path $Root "apps\agent")
$targets = @(
  @{ GOOS = "linux"; GOARCH = "amd64"; Ext = "" },
  @{ GOOS = "linux"; GOARCH = "arm64"; Ext = "" },
  @{ GOOS = "darwin"; GOARCH = "amd64"; Ext = "" },
  @{ GOOS = "darwin"; GOARCH = "arm64"; Ext = "" },
  @{ GOOS = "windows"; GOARCH = "amd64"; Ext = ".exe" }
)
foreach ($t in $targets) {
  $name = "fleetdeck-agent_${Version}_$($t.GOOS)_$($t.GOARCH)$($t.Ext)"
  Write-Host "Building $name"
  $env:CGO_ENABLED = "0"
  $env:GOOS = $t.GOOS
  $env:GOARCH = $t.GOARCH
  go build -ldflags="-s -w" -o (Join-Path $Out $name) ./cmd/fleetdeck-agent
}
Remove-Item Env:GOOS -ErrorAction SilentlyContinue
Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
Pop-Location

Get-ChildItem $Out -Filter "fleetdeck-agent_*" | ForEach-Object {
  Get-FileHash $_.FullName -Algorithm SHA256 | ForEach-Object {
    "$($_.Hash.ToLower())  $($_.Path | Split-Path -Leaf)"
  }
} | Set-Content -Encoding ascii (Join-Path $Out "SHA256SUMS")

Write-Host "Artifacts in $Out"
Write-Host "Optional: gpg --detach-sign --armor SHA256SUMS"

if (-not $NoCdn) {
  Write-Host "Publishing CDN bundle for $Version (auto-uploads when R2_* are set)..."
  & (Join-Path $Root "scripts\publish-cdn.ps1") -Version $Version
}
