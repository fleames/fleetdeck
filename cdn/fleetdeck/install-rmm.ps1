#Requires -Version 5.1
<#
.SYNOPSIS
  Install FleetDeck Windows RMM agent, enable RDP, and create a local RDP user.

.EXAMPLE
  # Preferred (panel generates this): save script then run — avoids iex scriptblock dumps.
  $s = Join-Path $env:TEMP 'install-rmm.ps1'
  Invoke-WebRequest 'https://agents.example.com/install-rmm.ps1' -OutFile $s -UseBasicParsing
  & $s -Token 'TOKEN' -ApiUrl 'https://agents.example.com'
#>
param(
  [string]$Token = $env:FLEETDECK_RMM_ENROLLMENT_TOKEN,
  [string]$ApiUrl = $env:FLEETDECK_URL,
  [string]$CdnBase = $env:FLEETDECK_CDN,
  [string]$Channel = $(if ($env:FLEETDECK_CHANNEL) { $env:FLEETDECK_CHANNEL } else { "latest" }),
  [string]$InstallDir = "$env:ProgramFiles\FleetDeck\RMM",
  [string]$StateDir = "$env:ProgramData\FleetDeck\RMM",
  [string]$RdpUser = $(if ($env:FLEETDECK_RDP_USER) { $env:FLEETDECK_RDP_USER } else { "fleetdeck" }),
  [string]$RdpPassword = $env:FLEETDECK_RDP_PASSWORD,
  [switch]$SkipRdpUser
)

$ErrorActionPreference = "Stop"
try {
  [Net.ServicePointManager]::SecurityProtocol =
    [Net.SecurityProtocolType]::Tls12 -bor [Net.SecurityProtocolType]::Tls11 -bor [Net.SecurityProtocolType]::Tls
} catch { }

function Write-Step([string]$Message) {
  Write-Host ""
  Write-Host "==> $Message" -ForegroundColor Cyan
}

function New-FleetDeckPassword {
  # Avoid shell/URL-hostile characters (%, &, =, ?, etc.) that can corrupt the secret
  # when passed through install args, query strings, or config files.
  $chars = (48..57) + (65..90) + (97..122) + (33,35,36,42,43,45,64) # ! # $ * + - @
  -join (1..20 | ForEach-Object { [char]$chars[(Get-Random -Maximum $chars.Count)] })
}

function Get-WindowsEditionId {
  try {
    return (Get-ItemProperty -Path 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion' -Name EditionID -ErrorAction Stop).EditionID
  } catch {
    return ""
  }
}

function Test-WindowsSupportsRdpHost {
  $edition = Get-WindowsEditionId
  # Home / S-mode style editions lack the Remote Desktop Users group and RDP host.
  $homeLike = @(
    'Core', 'CoreN', 'CoreSingleLanguage', 'CoreCountrySpecific',
    'Home', 'HomeN', 'HomeBasic', 'HomePremium', 'CoreConnected', 'PPIPro'
  )
  if ($edition -and ($homeLike -contains $edition)) {
    return $false
  }
  try {
    $null = Get-LocalGroup -SID 'S-1-5-32-555' -ErrorAction Stop
    return $true
  } catch {
    return $false
  }
}

function Enable-FleetDeckRdp {
  Write-Step "Enabling Remote Desktop"
  if (-not (Test-WindowsSupportsRdpHost)) {
    $edition = Get-WindowsEditionId
    Write-Error @"
This PC cannot host Remote Desktop (edition='$edition').
FleetDeck browser/native RDP needs Windows Pro, Enterprise, Education, or Windows Server — not Home.
Upgrade Windows, then re-run this installer.
"@
  }
  Set-ItemProperty -Path 'HKLM:\System\CurrentControlSet\Control\Terminal Server' -Name 'fDenyTSConnections' -Value 0 -Force
  # Keep NLA on (Guacamole / mstsc both support it with credentials).
  Set-ItemProperty -Path 'HKLM:\System\CurrentControlSet\Control\Terminal Server\WinStations\RDP-Tcp' -Name 'UserAuthentication' -Value 1 -Force -ErrorAction SilentlyContinue
  try {
    Enable-NetFirewallRule -DisplayGroup "Remote Desktop" -ErrorAction Stop
  } catch {
    # Localized firewall group name — enable by rule program / service instead.
    try {
      Get-NetFirewallRule -ErrorAction SilentlyContinue |
        Where-Object { $_.Group -like '*Remote Desktop*' -or $_.DisplayGroup -like '*Remote Desktop*' -or $_.Name -like 'RemoteDesktop*' } |
        Enable-NetFirewallRule -ErrorAction SilentlyContinue
    } catch { }
    netsh advfirewall firewall set rule group="remote desktop" new enable=Yes 2>$null | Out-Null
  }
  $svc = Get-Service -Name TermService -ErrorAction SilentlyContinue
  if ($svc -and $svc.Status -ne 'Running') {
    Start-Service TermService
  }
  Enable-FleetDeckRdpConcurrentSessions
}

function Test-WindowsServerSku {
  try {
    $pt = (Get-ItemProperty -Path 'HKLM:\SYSTEM\CurrentControlSet\Control\ProductOptions' -Name ProductType -ErrorAction Stop).ProductType
    if ($pt -eq 'ServerNT' -or $pt -eq 'LanmanNT') { return $true }
  } catch { }
  $edition = Get-WindowsEditionId
  $name = ""
  try {
    $name = (Get-ItemProperty -Path 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion' -Name ProductName -ErrorAction SilentlyContinue).ProductName
  } catch { }
  $blob = ("$edition $name").ToLowerInvariant()
  return ($blob -match 'server')
}

function Enable-FleetDeckRdpConcurrentSessions {
  # Windows client (Pro/Enterprise workstation): one interactive desktop only.
  # Concurrent RDP + console without takeover requires Windows Server (admin RDP or RDSH).
  # Do NOT patch termsrv / install RDP Wrapper — unsupported and ToS-violating.
  if (-not (Test-WindowsServerSku)) {
    Write-Host "Windows client SKU detected — concurrent RDP with another logged-on user is not supported by Microsoft."
    Write-Host "FleetDeck will not disconnect local users. Connect may show Windows takeover prompts while someone is at the console."
    return
  }
  Write-Step "Configuring Terminal Services for concurrent sessions (Windows Server)"
  # Supported: allow more than one session per user when reconnecting under the same account.
  # Distinct users (e.g. console operator + fleetdeck) already work under Server admin RDP limits.
  New-Item -Path 'HKLM:\SOFTWARE\Policies\Microsoft\Windows NT\Terminal Services' -Force | Out-Null
  Set-ItemProperty -Path 'HKLM:\SYSTEM\CurrentControlSet\Control\Terminal Server' -Name 'fSingleSessionPerUser' -Value 0 -Type DWord -Force
  Set-ItemProperty -Path 'HKLM:\SOFTWARE\Policies\Microsoft\Windows NT\Terminal Services' -Name 'fSingleSessionPerUser' -Value 0 -Type DWord -Force
  Write-Host "fSingleSessionPerUser=0 applied. More than two concurrent RDP users still needs RD Session Host + RDS CALs."
}

function Ensure-FleetDeckRdpUser([string]$User, [string]$Password, [string]$CredFile) {
  Write-Step "Creating RDP user '$User'"
  $secure = ConvertTo-SecureString $Password -AsPlainText -Force
  $existing = Get-LocalUser -Name $User -ErrorAction SilentlyContinue
  if ($existing) {
    Write-Host "User already exists — resetting password."
    Set-LocalUser -Name $User -Password $secure -PasswordNeverExpires $true
  } else {
    New-LocalUser -Name $User -Password $secure -FullName "FleetDeck RDP" -Description "FleetDeck remote desktop account" -PasswordNeverExpires | Out-Null
  }

  try {
    Set-LocalUser -Name $User -PasswordNeverExpires $true
  } catch { }

  # Resolve "Remote Desktop Users" by well-known SID (works on non-English Windows).
  # SID S-1-5-32-555 = BUILTIN\Remote Desktop Users
  $rdpGroup = $null
  try {
    $rdpGroup = Get-LocalGroup -SID 'S-1-5-32-555' -ErrorAction Stop
  } catch {
    Write-Error @"
Could not find the Remote Desktop Users group (SID S-1-5-32-555).
This usually means Windows Home (RDP host not available) or a locked-down image.
Use Windows Pro/Enterprise/Education/Server, then re-run install.
"@
  }

  $already = $false
  try {
    $already = [bool](Get-LocalGroupMember -Group $rdpGroup.Name -ErrorAction SilentlyContinue |
      Where-Object { $_.Name -eq $User -or $_.Name -like "*\$User" -or $_.Name -eq "$env:COMPUTERNAME\$User" })
  } catch { }

  if (-not $already) {
    try {
      Add-LocalGroupMember -Group $rdpGroup.Name -Member $User -ErrorAction Stop
    } catch {
      # Fallback for older images / odd localizations
      $netName = $rdpGroup.Name
      $out = & net.exe localgroup $netName $User /add 2>&1
      if ($LASTEXITCODE -ne 0) {
        Write-Error "Failed to add '$User' to '$netName': $out"
      }
    }
    Write-Host "Added '$User' to '$($rdpGroup.Name)'."
  } else {
    Write-Host "'$User' already in '$($rdpGroup.Name)'."
  }

  # Avoid account lock / expired password prompts on first RDP.
  # Guacamole maps UF_PASSWORD_EXPIRED / "must change at next logon" to
  # "Credentials expired." even when the stored password is correct.
  try {
    & net.exe user $User /active:yes /expires:never /logonpasswordchg:no | Out-Null
  } catch { }
  try {
    $ads = [ADSI]"WinNT://./$User,user"
    if ($null -ne $ads.PasswordExpired) {
      $ads.PasswordExpired = 0
      $ads.SetInfo()
    }
  } catch { }
  try {
    Set-LocalUser -Name $User -PasswordNeverExpires $true -ErrorAction SilentlyContinue
  } catch { }

  $credLines = @(
    "FleetDeck RDP credentials (created by install-rmm.ps1)",
    "username=$User",
    "password=$Password",
    "computer=$env:COMPUTERNAME",
    "created=$(Get-Date -Format o)"
  )
  Write-RestrictedFile -Path $CredFile -Lines $credLines
}

function Write-RestrictedFile([string]$Path, [string[]]$Lines) {
  # An earlier install could have removed inheritance and then failed to grant
  # Administrators (localized Windows), leaving the file unreadable/unwritable.
  if (Test-Path -LiteralPath $Path) {
    try { & takeown.exe /F $Path 2>$null | Out-Null } catch { }
    try { & icacls.exe $Path /reset 2>$null | Out-Null } catch { }
    Remove-Item -LiteralPath $Path -Force -ErrorAction SilentlyContinue
  }
  $dir = Split-Path -Parent $Path
  if ($dir -and -not (Test-Path -LiteralPath $dir)) {
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
  }
  Set-Content -LiteralPath $Path -Value $Lines -Encoding ASCII -Force
  # SIDs: BUILTIN\Administrators + NT AUTHORITY\SYSTEM (language-independent)
  & icacls.exe $Path /inheritance:r | Out-Null
  & icacls.exe $Path /grant:r "*S-1-5-32-544:(F)" "*S-1-5-18:(F)" | Out-Null
}

# --- require elevation ---
$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).
  IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
  Write-Error "Run elevated PowerShell as Administrator."
}

if (-not $Token) {
  Write-Error "Enrollment token required (-Token or FLEETDECK_RMM_ENROLLMENT_TOKEN)."
}

# Infer API URL from the host that served this script when possible.
if (-not $ApiUrl -and $PSScriptRoot -eq '' -and $MyInvocation.MyCommand.Path -like 'http*') {
  # no-op; irm|iex has no path
}
if (-not $ApiUrl) {
  $ApiUrl = "https://agents.example.com"
}
$ApiUrl = $ApiUrl.TrimEnd('/')

if (-not $CdnBase) { $CdnBase = "https://cdn.example.com/fleetdeck" }
$CdnBase = $CdnBase.TrimEnd('/')

New-Item -ItemType Directory -Force -Path $InstallDir, $StateDir | Out-Null

# --- RDP user ---
$createdPassword = $false
if (-not $SkipRdpUser) {
  if (-not $RdpPassword) {
    $RdpPassword = New-FleetDeckPassword
    $createdPassword = $true
  }
  if ($RdpUser -match '[\\/]') {
    Write-Error "RdpUser must be a local account name (no domain prefix)."
  }
  Enable-FleetDeckRdp
  $credFile = Join-Path $StateDir "rdp-credentials.txt"
  Ensure-FleetDeckRdpUser -User $RdpUser -Password $RdpPassword -CredFile $credFile
} else {
  Write-Host "Skipping RDP user creation (-SkipRdpUser)."
}

# --- download agent ---
function Get-FleetDeckBinary([string]$Url, [string]$OutFile) {
  $dir = Split-Path -Parent $OutFile
  if ($dir -and -not (Test-Path -LiteralPath $dir)) {
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
  }
  $tmp = Join-Path $env:TEMP ("fleetdeck-rmm-" + [guid]::NewGuid().ToString("n") + ".bin")
  $attempts = 3
  $last = $null
  for ($a = 1; $a -le $attempts; $a++) {
    try {
      Write-Host "  attempt $a/$attempts"
      Remove-Item -LiteralPath $tmp -Force -ErrorAction SilentlyContinue
      $curl = Get-Command curl.exe -ErrorAction SilentlyContinue
      if ($curl) {
        $code = & curl.exe -sSL --retry 2 --connect-timeout 20 --max-time 300 `
          -w "%{http_code}" -o $tmp $Url
        if ($LASTEXITCODE -ne 0) { throw "curl exit $LASTEXITCODE (http=$code)" }
        if ("$code" -ne "200") { throw "HTTP $code" }
      } else {
        $resp = Invoke-WebRequest -Uri $Url -OutFile $tmp -UseBasicParsing -TimeoutSec 300 -PassThru
        if ($resp.StatusCode -ne 200) { throw "HTTP $($resp.StatusCode)" }
      }
      if (-not (Test-Path -LiteralPath $tmp)) { throw "download produced no file" }
      $len = (Get-Item -LiteralPath $tmp).Length
      Write-Host "  got $len bytes"
      if ($len -lt 100000) { throw "download too small ($len bytes) — likely HTML/404" }
      $fs = [IO.File]::OpenRead($tmp)
      try {
        $b0 = $fs.ReadByte(); $b1 = $fs.ReadByte()
      } finally { $fs.Close() }
      if ($b0 -ne 0x4D -or $b1 -ne 0x5A) { throw "not a Windows PE binary (missing MZ header)" }
      Copy-Item -LiteralPath $tmp -Destination $OutFile -Force
      Remove-Item -LiteralPath $tmp -Force -ErrorAction SilentlyContinue
      return
    } catch {
      $last = $_.Exception.Message
      if (-not $last) { $last = "$_" }
      Write-Host "  failed: $last" -ForegroundColor DarkYellow
      Remove-Item -LiteralPath $tmp -Force -ErrorAction SilentlyContinue
      if ($a -lt $attempts) { Start-Sleep -Seconds (2 * $a) }
    }
  }
  throw $last
}

Write-Step "Downloading fleetdeck-rmm"
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$binPath = Join-Path $InstallDir "fleetdeck-rmm.exe"
$svcName = "FleetDeckRMM"

function Test-FleetDeckPe([string]$Path) {
  if (-not (Test-Path -LiteralPath $Path)) { return $false }
  if ((Get-Item -LiteralPath $Path).Length -lt 100000) { return $false }
  $fs = [IO.File]::OpenRead($Path)
  try {
    $b0 = $fs.ReadByte(); $b1 = $fs.ReadByte()
  } finally { $fs.Close() }
  return ($b0 -eq 0x4D -and $b1 -eq 0x5A)
}

# Native stderr under $ErrorActionPreference=Stop becomes terminating; swallow no-ops.
function Invoke-FleetDeckNativeQuiet {
  param([Parameter(Mandatory)][string]$CommandLine)
  $prev = $ErrorActionPreference
  $ErrorActionPreference = 'Continue'
  try {
    cmd /c "$CommandLine >nul 2>&1" | Out-Null
  } finally {
    $ErrorActionPreference = $prev
  }
}

function Stop-FleetDeckRmmProcessBestEffort {
  Get-Process -Name 'fleetdeck-rmm' -ErrorAction SilentlyContinue |
    Stop-Process -Force -ErrorAction SilentlyContinue
  # "process not found" must not abort install (common when service already stopped).
  Invoke-FleetDeckNativeQuiet 'taskkill /F /IM fleetdeck-rmm.exe'
}

# Unlock Program Files binary: service/process holds fleetdeck-rmm.exe open on re-install.
function Stop-FleetDeckRmmForReplace {
  $wasRunning = $false
  $svc = Get-Service -Name $svcName -ErrorAction SilentlyContinue
  if ($svc) {
    if ($svc.Status -ne 'Stopped') { $wasRunning = $true }
    Write-Host "Stopping service '$svcName' so the binary can be replaced..."
    Stop-Service -Name $svcName -Force -ErrorAction SilentlyContinue
    for ($i = 0; $i -lt 20; $i++) {
      $svc = Get-Service -Name $svcName -ErrorAction SilentlyContinue
      if (-not $svc -or $svc.Status -eq 'Stopped') { break }
      Start-Sleep -Milliseconds 500
    }
  }
  Stop-FleetDeckRmmProcessBestEffort
  Start-Sleep -Seconds 1
  return $wasRunning
}

function Start-FleetDeckRmmBestEffort {
  $svc = Get-Service -Name $svcName -ErrorAction SilentlyContinue
  if ($svc -and $svc.Status -ne 'Running') {
    Write-Host "Best-effort restart of '$svcName' after failed download..."
    try { Start-Service -Name $svcName -ErrorAction SilentlyContinue } catch { }
    Invoke-FleetDeckNativeQuiet "sc.exe start $svcName"
  }
}

# Reuse a previously downloaded agent only if it matches CDN SHA256 (when available).
$reuse = $false
if (Test-FleetDeckPe $binPath) {
  try {
    $sumUrl = "$CdnBase/$Channel/SHA256SUMS"
    $sums = (Invoke-WebRequest -Uri $sumUrl -UseBasicParsing -TimeoutSec 30).Content
    $line = ($sums -split "`n" | Where-Object { $_ -match "windows-amd64|fleetdeck-rmm\.exe" } | Select-Object -First 1)
    if ($line) {
      $expected = ($line -split "\s+")[0].ToLower()
      $actual = (Get-FileHash -LiteralPath $binPath -Algorithm SHA256).Hash.ToLower()
      if ($expected -eq $actual) {
        $reuse = $true
        Write-Host "Using existing agent at $binPath (SHA256 OK)"
      } else {
        Write-Host "Existing agent is outdated (SHA256 mismatch) — re-downloading."
      }
    } else {
      $reuse = $true
      Write-Host "Using existing agent at $binPath ($((Get-Item -LiteralPath $binPath).Length) bytes)"
    }
  } catch {
    $reuse = $true
    Write-Host "Using existing agent at $binPath ($((Get-Item -LiteralPath $binPath).Length) bytes)"
  }
}

if (-not $reuse) {
  $rmmWasRunning = Stop-FleetDeckRmmForReplace
  if (Test-Path -LiteralPath $binPath) {
    Remove-Item -LiteralPath $binPath -Force -ErrorAction SilentlyContinue
  }
  # Prefer extensionless CDN key first — Cloudflare custom-domain cache has held
  # stale fleetdeck-rmm.exe while windows-amd64 stayed current. Then .exe, then API.
  $candidates = @(
    "$CdnBase/$Channel/windows-amd64",
    "$CdnBase/$Channel/fleetdeck-rmm.exe",
    "$ApiUrl/install/rmm/windows/amd64"
  )
  $downloaded = $false
  $errors = @()
  try {
    foreach ($url in $candidates) {
      try {
        Write-Host "Trying $url"
        Get-FleetDeckBinary -Url $url -OutFile $binPath
        $downloaded = $true
        Write-Host "Downloaded $((Get-Item -LiteralPath $binPath).Length) bytes -> $binPath"
        break
      } catch {
        $msg = $_.Exception.Message
        if (-not $msg) { $msg = "$_" }
        $errors += "$url -> $msg"
        Write-Host "  giving up on this URL" -ForegroundColor DarkYellow
      }
    }
    if (-not $downloaded) {
      $hint = @"
Could not download fleetdeck-rmm.exe.
$($errors -join "`n")

Manual check on this PC:
  curl.exe -I https://cdn.tarkovbot.com/fleetdeck/latest/fleetdeck-rmm.exe
  curl.exe -I https://cdn.tarkovbot.com/fleetdeck/latest/windows-amd64
  curl.exe -I https://agents.tarkovbot.com/install/rmm/windows/amd64

Or download manually then re-run install:
  curl.exe -L -o `"$env:ProgramFiles\FleetDeck\RMM\fleetdeck-rmm.exe`" https://cdn.tarkovbot.com/fleetdeck/latest/fleetdeck-rmm.exe
"@
      throw $hint
    }
  } catch {
    if ($rmmWasRunning) { Start-FleetDeckRmmBestEffort }
    throw
  }
}

# --- enroll + service ---
Write-Step "Enrolling with FleetDeck ($ApiUrl)"
# Install commands always carry a fresh enrollment token. Clear local creds so we
# never stick on an orphan "Already enrolled" state after the host was removed
# from the panel (old agent binaries skip API verification).
$credJson = Join-Path $StateDir "credentials.json"
if (Test-Path -LiteralPath $credJson) {
  Write-Host "Clearing previous credentials to enroll with the new token..."
  Remove-Item -LiteralPath $credJson -Force -ErrorAction SilentlyContinue
}
$enrollArgs = @("-api", $ApiUrl, "-token", $Token, "-state-dir", $StateDir, "-enroll")
if (-not $SkipRdpUser) {
  $enrollArgs += @("-rdp-user", $RdpUser, "-rdp-password", $RdpPassword)
}
& $binPath @enrollArgs
if ($LASTEXITCODE -ne 0) {
  Write-Error "Enrollment failed. Generate a new install command from the FleetDeck panel and retry."
}

Write-Step "Installing Windows service '$svcName'"
$existing = Get-Service -Name $svcName -ErrorAction SilentlyContinue
if ($existing) {
  Write-Host "Removing existing service..."
  Stop-Service -Name $svcName -Force -ErrorAction SilentlyContinue
  Stop-FleetDeckRmmProcessBestEffort
  Invoke-FleetDeckNativeQuiet "sc.exe delete $svcName"
  Start-Sleep -Seconds 2
  # Wait until SCM finishes deleting
  for ($i = 0; $i -lt 15; $i++) {
    if (-not (Get-Service -Name $svcName -ErrorAction SilentlyContinue)) { break }
    Start-Sleep -Seconds 1
  }
}

# BinaryPathName must be one string: quoted exe path + args (handles "Program Files").
$binPathName = '"{0}" -service -api "{1}" -state-dir "{2}"' -f $binPath, $ApiUrl.TrimEnd('/'), $StateDir
try {
  New-Service -Name $svcName `
    -BinaryPathName $binPathName `
    -DisplayName "FleetDeck RMM Agent" `
    -Description "FleetDeck Windows RMM agent (outbound RDP tunnel)" `
    -StartupType Automatic | Out-Null
} catch {
  Write-Host "New-Service failed ($($_.Exception.Message)); trying sc.exe..."
  $createOut = & sc.exe create $svcName binPath= $binPathName start= auto DisplayName= "FleetDeck RMM Agent" 2>&1
  if ($LASTEXITCODE -ne 0) {
    Write-Error "Could not create service '$svcName'. sc.exe said: $createOut"
  }
  & sc.exe description $svcName "FleetDeck Windows RMM agent (outbound RDP tunnel)" | Out-Null
}

$created = Get-Service -Name $svcName -ErrorAction SilentlyContinue
if (-not $created) {
  Write-Error "Service '$svcName' was not created. BinaryPathName was: $binPathName"
}

try {
  Start-Service -Name $svcName -ErrorAction Stop
} catch {
  Write-Host "Start-Service failed; checking service configuration..."
  & sc.exe qc $svcName
  & sc.exe start $svcName
  Start-Sleep -Seconds 2
  $created = Get-Service -Name $svcName -ErrorAction SilentlyContinue
  if (-not $created -or $created.Status -ne 'Running') {
    Write-Error "Service '$svcName' exists but did not start. Run: sc.exe qc $svcName"
  }
}

$created = Get-Service -Name $svcName
Write-Host "Service status: $($created.Status)"

Write-Host ""
Write-Host "========================================" -ForegroundColor Green
Write-Host " FleetDeck RMM installed" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host "Service : $svcName ($($created.Status))"
Write-Host "State   : $StateDir"
if (-not $SkipRdpUser) {
  Write-Host ""
  Write-Host "RDP login (also sent to FleetDeck for one-click Connect):" -ForegroundColor Yellow
  Write-Host "  Username : $RdpUser"
  Write-Host "  Password : $RdpPassword"
  Write-Host "  Saved to : $(Join-Path $StateDir 'rdp-credentials.txt') (Administrators only)"
  if ($createdPassword) {
    Write-Host "  Password is stored encrypted in FleetDeck for auto-connect."
  }
}
Write-Host ""
