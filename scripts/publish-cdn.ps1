#Requires -Version 5.1
# Publish FleetDeck agent artifacts for https://cdn.tarkovbot.com/fleetdeck/
# Uploads to R2 automatically when R2_* credentials are set in .env (or the environment).
# Use -NoUpload to skip; -Upload to force upload even if detection is unclear.
param(
  [string]$Version = "0.4.2-dev",
  [string]$Channel = "latest",
  [string]$ApiUrl = "https://agents.tarkovbot.com",
  [string]$OutRoot = "",
  [switch]$Upload,
  [switch]$NoUpload,
  [switch]$SkipVerify
)
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
if (-not $OutRoot) { $OutRoot = Join-Path $Root "dist\cdn\fleetdeck" }

$EnvFile = Join-Path $Root ".env"

function Get-EnvValue([string]$Path, [string]$Key) {
  if (-not (Test-Path -LiteralPath $Path)) { return $null }
  $line = Get-Content -LiteralPath $Path | Where-Object { $_ -match ("^\s*" + [regex]::Escape($Key) + "\s*=") } | Select-Object -First 1
  if (-not $line) { return $null }
  return ($line -replace "^[^=]*=", "").Trim().Trim('"').Trim("'")
}

function Resolve-R2Var([string]$Name) {
  $v = [Environment]::GetEnvironmentVariable($Name)
  if ($v) { return $v.Trim() }
  return Get-EnvValue $EnvFile $Name
}

function Test-R2Configured {
  $required = @("R2_ACCOUNT_ID", "R2_ACCESS_KEY_ID", "R2_SECRET_ACCESS_KEY", "R2_BUCKET")
  foreach ($name in $required) {
    $v = Resolve-R2Var $name
    if (-not $v) { return $false }
  }
  return $true
}

$ChannelDir = Join-Path $OutRoot $Channel
$VersionDir = Join-Path $OutRoot $Version
New-Item -ItemType Directory -Force -Path $ChannelDir, $VersionDir | Out-Null

# Build linux agents into release dir first if missing
$ReleaseDir = Join-Path $Root "dist\agent\$Version"
if (-not (Test-Path (Join-Path $ReleaseDir "fleetdeck-agent_${Version}_linux_amd64"))) {
  Write-Host "Building agent release $Version..."
  & (Join-Path $Root "scripts\release-agent.ps1") -Version $Version -NoCdn
}

Copy-Item (Join-Path $ReleaseDir "fleetdeck-agent_${Version}_linux_amd64") (Join-Path $ChannelDir "linux-amd64") -Force
Copy-Item (Join-Path $ReleaseDir "fleetdeck-agent_${Version}_linux_arm64") (Join-Path $ChannelDir "linux-arm64") -Force
Copy-Item (Join-Path $ReleaseDir "fleetdeck-agent_${Version}_linux_amd64") (Join-Path $VersionDir "linux-amd64") -Force
Copy-Item (Join-Path $ReleaseDir "fleetdeck-agent_${Version}_linux_arm64") (Join-Path $VersionDir "linux-arm64") -Force

$InstallSrc = Join-Path $Root "cdn\fleetdeck\install.sh"
Copy-Item $InstallSrc (Join-Path $OutRoot "install.sh") -Force
Copy-Item $InstallSrc (Join-Path $ChannelDir "install.sh") -Force
Copy-Item $InstallSrc (Join-Path $VersionDir "install.sh") -Force

$InstallUserSrc = Join-Path $Root "cdn\fleetdeck\install-user.sh"
Copy-Item $InstallUserSrc (Join-Path $OutRoot "install-user.sh") -Force
Copy-Item $InstallUserSrc (Join-Path $ChannelDir "install-user.sh") -Force
Copy-Item $InstallUserSrc (Join-Path $VersionDir "install-user.sh") -Force

$UpgradeSrc = Join-Path $Root "cdn\fleetdeck\upgrade.sh"
Copy-Item $UpgradeSrc (Join-Path $OutRoot "upgrade.sh") -Force
Copy-Item $UpgradeSrc (Join-Path $ChannelDir "upgrade.sh") -Force
Copy-Item $UpgradeSrc (Join-Path $VersionDir "upgrade.sh") -Force

$configJson = "{`"api_url`":`"$ApiUrl`"}"
[System.IO.File]::WriteAllText((Join-Path $OutRoot "config.json"), $configJson)
[System.IO.File]::WriteAllText((Join-Path $ChannelDir "config.json"), $configJson)
[System.IO.File]::WriteAllText((Join-Path $VersionDir "config.json"), $configJson)

function Write-Sums([string]$Dir) {
  Get-ChildItem $Dir -File | Where-Object { $_.Name -like "linux-*" } | ForEach-Object {
    $h = (Get-FileHash $_.FullName -Algorithm SHA256).Hash.ToLower()
    "$h  $($_.Name)"
  } | Set-Content -Encoding ascii (Join-Path $Dir "SHA256SUMS")
}
Write-Sums $ChannelDir
Write-Sums $VersionDir

@"
# FleetDeck CDN layout

Upload the contents of this folder to R2 (prefix fleetdeck/) so these URLs work:

- https://cdn.tarkovbot.com/fleetdeck/install.sh
- https://cdn.tarkovbot.com/fleetdeck/install-user.sh
- https://cdn.tarkovbot.com/fleetdeck/upgrade.sh
- https://cdn.tarkovbot.com/fleetdeck/config.json
- https://cdn.tarkovbot.com/fleetdeck/latest/linux-amd64
- https://cdn.tarkovbot.com/fleetdeck/latest/linux-arm64
- https://cdn.tarkovbot.com/fleetdeck/latest/SHA256SUMS

With R2_* set in .env, publish-cdn.ps1 uploads automatically.
Or force: .\scripts\publish-cdn.ps1 -Upload
Skip: .\scripts\publish-cdn.ps1 -NoUpload

config.json api_url = $ApiUrl

VPS install (FleetDeck PC must be online with Cloudflare Tunnel):

curl -fsSL https://cdn.tarkovbot.com/fleetdeck/install.sh | sudo bash -s -- --token 'TOKEN'

Seedbox / no sudo (user install under ~/.local/bin + ~/.fleetdeck):

curl -fsSL https://cdn.tarkovbot.com/fleetdeck/install.sh | bash -s -- --user --token 'TOKEN'

Manual upgrade (keeps credentials; for 0.4.0 hosts without panel agent.update):

curl -fsSL https://cdn.tarkovbot.com/fleetdeck/upgrade.sh | sudo bash

Panel Update agent pulls latest/linux-{arch} (and SHA256SUMS when present).
After shipping a new agent, re-run this script so CDN binaries + install.sh/upgrade.sh (update path units) stay in sync.

Generated: $(Get-Date -Format o)
"@ | Set-Content -Encoding utf8 (Join-Path $OutRoot "README-UPLOAD.txt")

Write-Host "CDN bundle ready at $OutRoot"
Get-ChildItem $OutRoot -Recurse -File | Select-Object FullName, Length

$r2Ready = Test-R2Configured
$shouldUpload = $false
if ($NoUpload -and $Upload) {
  Write-Warning 'Both Upload and NoUpload were set; NoUpload wins.'
}
if ($NoUpload) {
  $shouldUpload = $false
} elseif ($Upload) {
  $shouldUpload = $true
} elseif ($r2Ready) {
  $shouldUpload = $true
  Write-Host 'R2 credentials found - uploading CDN artifacts...'
}

if ($shouldUpload) {
  $uploadArgs = @{ SourceDir = $OutRoot }
  if ($SkipVerify) { $uploadArgs.SkipVerify = $true }
  & (Join-Path $Root "scripts\upload-cdn-r2.ps1") @uploadArgs
} elseif ($NoUpload) {
  Write-Host 'Skipped upload (NoUpload switch).'
} else {
  Write-Host 'No R2 credentials in .env - bundle left local only.'
  Write-Host 'Set R2_ACCOUNT_ID, R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY, R2_BUCKET (see .env.example),'
  Write-Host ('then re-run: .\scripts\publish-cdn.ps1 -Version ' + $Version)
  Write-Host 'Or force upload once credentials exist: .\scripts\publish-cdn.ps1 -Upload'
}
