#Requires -Version 5.1
<#
.SYNOPSIS
  Register a Windows Scheduled Task that starts FleetDeck at user logon.

.DESCRIPTION
  Creates task "FleetDeck-Autostart" for the current user. On logon it runs
  scripts\start.ps1 (which waits for Docker Engine, then docker compose up).

  Also enable Docker Desktop → Settings → General →
  "Start Docker Desktop when you log in" so the engine is available after reboot.
#>
param(
  [string]$TaskName = "FleetDeck-Autostart"
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

$Root = $ScriptDir
while ($Root) {
  if (Test-Path -LiteralPath (Join-Path $Root "deploy\docker-compose.yml")) { break }
  $parent = Split-Path -Parent $Root
  if (-not $parent -or $parent -eq $Root) {
    throw "Could not locate FleetDeck repo root (deploy/docker-compose.yml)."
  }
  $Root = $parent
}

$StartScript = Join-Path $Root "scripts\start.ps1"
if (-not (Test-Path -LiteralPath $StartScript)) {
  throw "Missing start script: $StartScript"
}

$psExe = Join-Path $env:SystemRoot "System32\WindowsPowerShell\v1.0\powershell.exe"
if (-not (Test-Path -LiteralPath $psExe)) {
  $psExe = "powershell.exe"
}

# -WindowStyle Hidden keeps logon quieter; logs still go to task history if enabled.
$arg = "-NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File `"$StartScript`""

$action = New-ScheduledTaskAction -Execute $psExe -Argument $arg -WorkingDirectory $Root
$trigger = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
$principal = New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType Interactive -RunLevel Limited
$settings = New-ScheduledTaskSettingsSet `
  -AllowStartIfOnBatteries `
  -DontStopIfGoingOnBatteries `
  -StartWhenAvailable `
  -ExecutionTimeLimit (New-TimeSpan -Hours 1) `
  -RestartCount 3 `
  -RestartInterval (New-TimeSpan -Minutes 1)

Register-ScheduledTask `
  -TaskName $TaskName `
  -Action $action `
  -Trigger $trigger `
  -Principal $principal `
  -Settings $settings `
  -Description "Start FleetDeck Docker Compose stack (scripts/start.ps1) after user logon. Waits for Docker Engine." `
  -Force | Out-Null

Write-Host "Registered scheduled task: $TaskName"
Write-Host "  Runs at logon for: $env:USERNAME"
Write-Host "  Working directory: $Root"
Write-Host "  Action: $psExe $arg"
Write-Host ""
Write-Host "Also enable Docker Desktop > Settings > General >"
Write-Host '  "Start Docker Desktop when you log in"'
Write-Host ""
Write-Host "Verify:"
Write-Host "  Get-ScheduledTask -TaskName '$TaskName'"
Write-Host "  Start-ScheduledTask -TaskName '$TaskName'   # dry-run without reboot"
Write-Host "  schtasks /Query /TN `"$TaskName`" /V /FO LIST"
Write-Host ""
Write-Host "Remove later: .\scripts\uninstall-autostart.ps1"
