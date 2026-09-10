#Requires -Version 5.1
param(
  [switch]$Build
)

$ErrorActionPreference = "Stop"

$ScriptDir = $PSScriptRoot
if (-not $ScriptDir) {
  if ($MyInvocation.MyCommand.Path) {
    $ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
  } else {
    $ScriptDir = (Get-Location).Path
  }
}

# Walk up until deploy/docker-compose.yml is found
$Root = $ScriptDir
while ($Root) {
  if (Test-Path -LiteralPath (Join-Path $Root "deploy\docker-compose.yml")) { break }
  $parent = Split-Path -Parent $Root
  if (-not $parent -or $parent -eq $Root) {
    throw "Could not locate FleetDeck repo root (deploy/docker-compose.yml)."
  }
  $Root = $parent
}

$EnvFile = Join-Path $Root ".env"
$ComposeFile = Join-Path $Root "deploy\docker-compose.yml"
Write-Host "Repo root: $Root"

function Get-EnvValue([string]$Path, [string]$Key) {
  if (-not (Test-Path -LiteralPath $Path)) { return $null }
  $line = Get-Content -LiteralPath $Path | Where-Object { $_ -match ("^\s*" + [regex]::Escape($Key) + "\s*=") } | Select-Object -First 1
  if (-not $line) { return $null }
  return ($line -replace "^[^=]*=", "").Trim().Trim('"').Trim("'")
}

function Wait-DockerEngine {
  param(
    [int]$TimeoutSeconds = 300,
    [int]$IntervalSeconds = 5
  )
  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  Write-Host "Waiting for Docker Engine (docker info)..."
  while ((Get-Date) -lt $deadline) {
    try {
      & docker info 1>$null 2>$null
      if ($LASTEXITCODE -eq 0) {
        Write-Host "Docker Engine is ready."
        return
      }
    } catch {
      # docker missing or not responding yet
    }
    Start-Sleep -Seconds $IntervalSeconds
  }
  throw "Docker Engine not ready after ${TimeoutSeconds}s. Start Docker Desktop (enable 'Start Docker Desktop when you log in') and retry."
}

Push-Location -LiteralPath $Root
try {
  Wait-DockerEngine

  $token = Get-EnvValue $EnvFile "CLOUDFLARE_TUNNEL_TOKEN"
  $composeArgs = @("--env-file", $EnvFile, "-f", $ComposeFile, "up", "-d")
  if ($Build) {
    $composeArgs = @("--env-file", $EnvFile, "-f", $ComposeFile, "up", "-d", "--build")
  }

  if ($token) {
    Write-Host "CLOUDFLARE_TUNNEL_TOKEN set - starting with tunnel profile"
    $composeArgs = @("--profile", "tunnel") + $composeArgs
  } else {
    Write-Host "No CLOUDFLARE_TUNNEL_TOKEN - starting without tunnel (local only)"
    Write-Host "  Run .\scripts\setup-cloudflare-tunnel.ps1 once for remote agents"
  }

  & docker compose @composeArgs
  if ($LASTEXITCODE -ne 0) { throw "docker compose failed ($LASTEXITCODE)" }

  if ($token) {
    $api = Get-EnvValue $EnvFile "API_PUBLIC_URL"
    if (-not $api) { $api = "https://agents.example.com" }
    Write-Host "Waiting for tunnel healthz..."
    $ok = $false
    for ($i = 0; $i -lt 30; $i++) {
      Start-Sleep -Seconds 2
      try {
        $r = Invoke-WebRequest -Uri "$api/healthz" -UseBasicParsing -TimeoutSec 5
        if ($r.StatusCode -ge 200 -and $r.StatusCode -lt 300) {
          Write-Host "OK $($r.StatusCode) $api/healthz"
          Write-Host $r.Content
          $ok = $true
          break
        }
      } catch {
        Write-Host ("  not ready yet ({0})" -f $_.Exception.Message)
      }
    }
    if (-not $ok) {
      Write-Host "Tunnel not healthy yet - check: docker compose --env-file .env -f deploy/docker-compose.yml --profile tunnel logs cloudflared"
    }
  }
} finally {
  Pop-Location
}
