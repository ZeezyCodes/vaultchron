<#
.SYNOPSIS
    Registers or unregisters the VaultChron daily Scheduled Task on Windows.

.DESCRIPTION
    Creates a user-level Windows Scheduled Task that executes vaultchron-run.ps1
    daily at the specified time without requiring administrative elevation.
    Mirrors deploy/vaultchron.timer (07:00:00 daily, Persistent=true).

.PARAMETER Time
    The daily trigger time in HH:mm format (default: "07:00", mirroring deploy/vaultchron.timer).

.PARAMETER TaskName
    The name of the Scheduled Task (default: "VaultChron").

.PARAMETER Unregister
    If specified, removes the Scheduled Task instead of registering it.
#>

[CmdletBinding()]
param(
    [string]$Time = "07:00",
    [string]$TaskName = "VaultChron",
    [switch]$Unregister
)

Set-StrictMode -Version Latest

if ($Unregister) {
    $existing = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
    if ($existing) {
        Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false
        Write-Host "Scheduled task '$TaskName' unregistered successfully."
    } else {
        Write-Host "Scheduled task '$TaskName' does not exist."
    }
    return
}

# Resolve wrapper script located alongside this script.
$wrapper = Join-Path $PSScriptRoot "vaultchron-run.ps1"
if (-not (Test-Path -LiteralPath $wrapper)) {
    Write-Error "Wrapper script not found: $wrapper"
    exit 1
}
$wrapperPath = [System.IO.Path]::GetFullPath($wrapper)

# Build action: powershell.exe -NoProfile -ExecutionPolicy Bypass -File "<wrapper>"
$action = New-ScheduledTaskAction -Execute "powershell.exe" -Argument "-NoProfile -ExecutionPolicy Bypass -File `"$wrapperPath`""

# Build daily trigger at specified time.
$trigger = New-ScheduledTaskTrigger -Daily -At $Time

# Configure settings: StartWhenAvailable (catch up missed runs), allow start on battery, 2-hour execution limit.
$settings = New-ScheduledTaskSettingsSet -StartWhenAvailable -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -ExecutionTimeLimit (New-TimeSpan -Hours 2)

# Principal: current user with Interactive logon (no admin elevation, no stored password).
$currentUser = $env:USERNAME
$principal = New-ScheduledTaskPrincipal -UserId $currentUser -LogonType Interactive

# Register scheduled task.
Register-ScheduledTask -TaskName $TaskName -Action $action -Trigger $trigger -Settings $settings -Principal $principal -Force | Out-Null

Write-Host "Scheduled task '$TaskName' registered successfully (Daily at $Time)."
Write-Host "To run it now manually: Start-ScheduledTask -TaskName '$TaskName'"
Write-Host "To check results:        Get-ScheduledTaskInfo -TaskName '$TaskName'"
