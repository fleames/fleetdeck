# Cross-compile FleetDeck RMM agent (Windows amd64) with checksums.
param(
  [string]$Version = "0.1.4-dev",
  [string]$AgentVersion = "0.4.7-dev",
  [switch]$NoCdn
)
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
$Out = Join-Path $Root "dist\rmm-agent\$Version"
New-Item -ItemType Directory -Force -Path $Out | Out-Null

Push-Location (Join-Path $Root "apps\rmm-agent")
Write-Host "Building fleetdeck-rmm_${Version}_windows_amd64.exe"
$env:CGO_ENABLED = "0"
$env:GOOS = "windows"
$env:GOARCH = "amd64"
go build -ldflags="-s -w" -o (Join-Path $Out "fleetdeck-rmm_${Version}_windows_amd64.exe") ./cmd/fleetdeck-rmm
Remove-Item Env:GOOS -ErrorAction SilentlyContinue
Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
Pop-Location

Get-ChildItem $Out -Filter "fleetdeck-rmm_*" | ForEach-Object {
  Get-FileHash $_.FullName -Algorithm SHA256 | ForEach-Object {
    "$($_.Hash.ToLower())  $($_.Path | Split-Path -Leaf)"
  }
} | Set-Content -Encoding ascii (Join-Path $Out "SHA256SUMS")

Write-Host "Artifacts in $Out"

if (-not $NoCdn) {
  Write-Host "Publishing CDN bundle (includes RMM windows-amd64 when present)..."
  & (Join-Path $Root "scripts\publish-cdn.ps1") -Version $AgentVersion -RmmVersion $Version
}
