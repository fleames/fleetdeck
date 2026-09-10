#Requires -Version 5.1
<#
.SYNOPSIS
  Remove the FleetDeck Windows logon Scheduled Task.
#>
param(
  [string]$TaskName = "FleetDeck-Autostart"
)

$ErrorActionPreference = "Stop"

$existing = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
if (-not $existing) {
  Write-Host "No scheduled task named '$TaskName' found — nothing to remove."
  exit 0
}

Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false
Write-Host "Removed scheduled task: $TaskName"
