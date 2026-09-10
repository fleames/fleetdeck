#Requires -Version 5.1
param(
  [string]$ApiToken = $env:CLOUDFLARE_API_TOKEN,
  [string]$AccountId = $env:CLOUDFLARE_ACCOUNT_ID,
  [string]$Hostname = "agents.example.com",
  [string]$TunnelName = "fleetdeck-home",
  [string]$OriginService = "http://api:8080",
  [switch]$SkipStart
)

$ErrorActionPreference = "Stop"

# Resolve repo root even when PSScriptRoot is empty (some cmd/.ps1 associations)
$ScriptDir = $PSScriptRoot
if (-not $ScriptDir) {
  if ($MyInvocation.MyCommand.Path) {
    $ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
  } else {
    $ScriptDir = (Get-Location).Path
  }
}
$Root = $ScriptDir
while ($Root) {
  if (Test-Path -LiteralPath (Join-Path $Root "deploy\docker-compose.yml")) { break }
  $parent = Split-Path -Parent $Root
  if (-not $parent -or $parent -eq $Root) {
    throw "Could not locate FleetDeck repo root from script path."
  }
  $Root = $parent
}
$EnvFile = Join-Path $Root ".env"

function Get-EnvValue([string]$Path, [string]$Key) {
  if (-not (Test-Path $Path)) { return $null }
  $line = Get-Content -LiteralPath $Path | Where-Object { $_ -match ("^\s*" + [regex]::Escape($Key) + "\s*=") } | Select-Object -First 1
  if (-not $line) { return $null }
  return ($line -replace "^[^=]*=", "").Trim().Trim('"').Trim("'")
}

function Write-TextFileRetry([string]$Path, [string[]]$Lines) {
  $tmp = "$Path.tmp.$PID"
  $text = ($Lines -join "`r`n") + "`r`n"
  $max = 12
  for ($i = 1; $i -le $max; $i++) {
    try {
      [System.IO.File]::WriteAllText($tmp, $text, [System.Text.UTF8Encoding]::new($false))
      [System.IO.File]::Copy($tmp, $Path, $true)
      Remove-Item -LiteralPath $tmp -Force -ErrorAction SilentlyContinue
      return
    } catch {
      if ($i -eq $max) {
        $fallback = Join-Path (Split-Path -Parent $Path) (".env.tunnel." + (Get-Date -Format "yyyyMMddHHmmss"))
        [System.IO.File]::WriteAllText($fallback, $text, [System.Text.UTF8Encoding]::new($false))
        throw ("Could not write '{0}' (locked). Wrote values to '{1}' instead. Close editors using .env, copy values over, then run .\scripts\start.ps1 -Build. Inner: {2}" -f $Path, $fallback, $_.Exception.Message)
      }
      Start-Sleep -Milliseconds (150 * $i)
    }
  }
}

function Set-EnvValues([string]$Path, [hashtable]$Pairs) {
  if (-not (Test-Path -LiteralPath $Path)) {
    $example = Join-Path $Root ".env.example"
    if (Test-Path -LiteralPath $example) {
      Copy-Item -LiteralPath $example -Destination $Path
    } else {
      New-Item -ItemType File -Path $Path | Out-Null
    }
  }
  $lines = @([System.IO.File]::ReadAllLines($Path))
  $seen = @{}
  $out = foreach ($line in $lines) {
    $matched = $false
    foreach ($key in $Pairs.Keys) {
      if ($line -match ("^\s*" + [regex]::Escape($key) + "\s*=")) {
        $matched = $true
        $seen[$key] = $true
        "$key=$($Pairs[$key])"
        break
      }
    }
    if (-not $matched) { $line }
  }
  foreach ($key in $Pairs.Keys) {
    if (-not $seen[$key]) {
      $out += ""
      $out += "$key=$($Pairs[$key])"
    }
  }
  Write-TextFileRetry -Path $Path -Lines $out
}

function Set-EnvValue([string]$Path, [string]$Key, [string]$Value) {
  Set-EnvValues -Path $Path -Pairs @{ $Key = $Value }
}

function Invoke-CF([string]$Method, [string]$Url, $Body = $null) {
  $headers = @{
    Authorization = "Bearer $ApiToken"
    "Content-Type" = "application/json"
  }
  $params = @{
    Method      = $Method
    Uri         = $Url
    Headers     = $headers
    ErrorAction = "Stop"
  }
  if ($null -ne $Body) {
    $params.Body = ($Body | ConvertTo-Json -Depth 10 -Compress)
  }
  $res = Invoke-RestMethod @params
  if (-not $res.success) {
    $err = ($res.errors | ConvertTo-Json -Compress)
    throw "Cloudflare API error: $err"
  }
  return $res.result
}

if (-not $ApiToken) {
  $ApiToken = Get-EnvValue $EnvFile "CLOUDFLARE_API_TOKEN"
}
if (-not $ApiToken) {
  Write-Host "Missing CLOUDFLARE_API_TOKEN."
  Write-Host ""
  Write-Host "Create a token at https://dash.cloudflare.com/profile/api-tokens"
  Write-Host "  Account -> Cloudflare Tunnel -> Edit"
  Write-Host "  Zone -> DNS -> Edit"
  Write-Host ""
  Write-Host "Then in PowerShell:"
  Write-Host '  $env:CLOUDFLARE_API_TOKEN = "YOUR_TOKEN"'
  Write-Host "  .\scripts\setup-cloudflare-tunnel.ps1"
  Write-Host ""
  Write-Host "Or from cmd.exe:"
  Write-Host "  scripts\setup-cloudflare-tunnel.cmd"
  exit 1
}

if (-not $AccountId) {
  $AccountId = Get-EnvValue $EnvFile "CLOUDFLARE_ACCOUNT_ID"
}
if (-not $AccountId) {
  Write-Host "Resolving Cloudflare account id..."
  $accounts = Invoke-CF GET "https://api.cloudflare.com/client/v4/accounts?per_page=50"
  if ($accounts -is [System.Array] -and $accounts.Count -gt 1) {
    Write-Host "Multiple accounts - set CLOUDFLARE_ACCOUNT_ID explicitly:"
    $accounts | ForEach-Object { Write-Host ("  {0}  {1}" -f $_.id, $_.name) }
    exit 1
  }
  if ($accounts -is [System.Array]) { $AccountId = $accounts[0].id } else { $AccountId = $accounts.id }
}
Write-Host "Account: $AccountId"

if ($Hostname -notmatch '^([^.]+)\.(.+)$') {
  throw "Hostname must look like agents.example.com"
}
$subdomain = $Matches[1]
$zoneName = $Matches[2]
$publicUrl = "https://$Hostname"

Write-Host "Looking up zone $zoneName..."
$zones = Invoke-CF GET "https://api.cloudflare.com/client/v4/zones?name=$([uri]::EscapeDataString($zoneName))"
if ($zones -is [System.Array]) { $zone = $zones | Select-Object -First 1 } else { $zone = $zones }
if (-not $zone) { throw "Zone '$zoneName' not found for this token." }
$zoneId = $zone.id
Write-Host "Zone: $zoneId"

Write-Host "Ensuring tunnel '$TunnelName'..."
$tunnels = Invoke-CF GET (
    "https://api.cloudflare.com/client/v4/accounts/$AccountId/cfd_tunnel?name=$([uri]::EscapeDataString($TunnelName))" +
    "&is_deleted=false"
  )
$tunnel = $null
if ($tunnels -is [System.Array]) {
  $tunnel = $tunnels | Where-Object { $_.name -eq $TunnelName -and -not $_.deleted_at } | Select-Object -First 1
} elseif ($tunnels -and $tunnels.name -eq $TunnelName) {
  $tunnel = $tunnels
}

$tunnelToken = $null
if (-not $tunnel) {
  $created = Invoke-CF POST "https://api.cloudflare.com/client/v4/accounts/$AccountId/cfd_tunnel" @{
    name       = $TunnelName
    config_src = "cloudflare"
  }
  $tunnel = $created
  $tunnelToken = $created.token
  Write-Host "Created tunnel $($tunnel.id)"
} else {
  Write-Host "Reusing tunnel $($tunnel.id)"
}

if (-not $tunnelToken) {
  $tunnelToken = Invoke-CF GET "https://api.cloudflare.com/client/v4/accounts/$AccountId/cfd_tunnel/$($tunnel.id)/token"
  if ($tunnelToken -isnot [string] -and $tunnelToken.token) {
    $tunnelToken = $tunnelToken.token
  }
}
if (-not $tunnelToken) {
  throw "Could not obtain tunnel run token from Cloudflare API."
}

Write-Host "Configuring ingress $Hostname -> $OriginService..."
Invoke-CF PUT "https://api.cloudflare.com/client/v4/accounts/$AccountId/cfd_tunnel/$($tunnel.id)/configurations" @{
  config = @{
    ingress = @(
      @{ hostname = $Hostname; service = $OriginService; originRequest = @{} }
      @{ service = "http_status:404" }
    )
  }
} | Out-Null

Write-Host "Ensuring DNS CNAME $Hostname -> $($tunnel.id).cfargotunnel.com (proxied)..."
$existing = Invoke-CF GET (
    "https://api.cloudflare.com/client/v4/zones/$zoneId/dns_records?type=CNAME" +
    "&name=$([uri]::EscapeDataString($Hostname))"
  )
$rec = $null
if ($existing -is [System.Array]) { $rec = $existing | Select-Object -First 1 }
elseif ($existing) { $rec = $existing }
$cnameTarget = "$($tunnel.id).cfargotunnel.com"
if ($rec) {
  Invoke-CF PUT "https://api.cloudflare.com/client/v4/zones/$zoneId/dns_records/$($rec.id)" @{
    type    = "CNAME"
    name    = $Hostname
    content = $cnameTarget
    proxied = $true
    ttl     = 1
  } | Out-Null
  Write-Host "Updated DNS record $($rec.id)"
} else {
  Invoke-CF POST "https://api.cloudflare.com/client/v4/zones/$zoneId/dns_records" @{
    type    = "CNAME"
    name    = $subdomain
    content = $cnameTarget
    proxied = $true
    ttl     = 1
  } | Out-Null
  Write-Host "Created DNS record"
}

Write-Host "Updating .env..."
Set-EnvValues -Path $EnvFile -Pairs @{
  CLOUDFLARE_ACCOUNT_ID   = $AccountId
  CLOUDFLARE_TUNNEL_TOKEN = "$tunnelToken"
  API_PUBLIC_URL          = $publicUrl
  RELAY_URL               = ""
  RELAY_TOKEN             = ""
}

Write-Host ""
Write-Host "Tunnel ready: $publicUrl"
Write-Host "  tunnel id : $($tunnel.id)"
Write-Host "  origin    : $OriginService"

if ($SkipStart) {
  Write-Host "SkipStart set - run: .\scripts\start.ps1"
  exit 0
}

$startScript = Join-Path $ScriptDir "start.ps1"
& powershell.exe -NoProfile -ExecutionPolicy Bypass -File $startScript -Build
if ($LASTEXITCODE -ne 0) {
  Write-Host "start.ps1 exited with $LASTEXITCODE - you can retry: .\scripts\start.ps1 -Build"
}
Write-Host ""
Write-Host "Verify: curl.exe -sS $publicUrl/healthz"
Write-Host "Then publish CDN: .\scripts\publish-cdn.ps1 -Upload"
