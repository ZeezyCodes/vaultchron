<#
.SYNOPSIS
    VaultChron Windows execution wrapper for Scheduled Tasks and manual runs.

.DESCRIPTION
    Sets process environment variables from %APPDATA%\vaultchron\env,
    navigates to the installation directory, and runs vaultchron.exe with
    the default configuration while logging UTF-8 output to
    %LOCALAPPDATA%\vaultchron\logs\vaultchron.log.
#>

Set-StrictMode -Version Latest

# Resolve installation directory.
if ($env:VAULTCHRON_HOME) {
    $installDir = $env:VAULTCHRON_HOME
} else {
    $archiveRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot "..\.."))
    if (Test-Path -LiteralPath (Join-Path $archiveRoot "vaultchron.exe")) {
        $installDir = $archiveRoot
    } else {
        $installDir = Join-Path $env:LOCALAPPDATA "vaultchron"
    }
}

$exePath = Join-Path $installDir "vaultchron.exe"
if (-not (Test-Path -LiteralPath $exePath)) {
    Write-Error "vaultchron.exe not found in $installDir"
    exit 1
}

# Optional environment file: %APPDATA%\vaultchron\env
$appDataDir = $env:APPDATA
if (-not $appDataDir) {
    $appDataDir = [Environment]::GetFolderPath([Environment+SpecialFolder]::ApplicationData)
}
$envFile = Join-Path $appDataDir "vaultchron\env"

if (Test-Path -LiteralPath $envFile) {
    Get-Content -LiteralPath $envFile | ForEach-Object {
        $line = $_.Trim()
        if ($line -and -not $line.StartsWith("#")) {
            $eqIndex = $line.IndexOf("=")
            if ($eqIndex -gt 0) {
                $key = $line.Substring(0, $eqIndex).Trim()
                $val = $line.Substring($eqIndex + 1).Trim()
                if (($val.StartsWith('"') -and $val.EndsWith('"')) -or ($val.StartsWith("'") -and $val.EndsWith("'"))) {
                    if ($val.Length -ge 2) {
                        $val = $val.Substring(1, $val.Length - 2)
                    }
                }
                [Environment]::SetEnvironmentVariable($key, $val, [EnvironmentVariableTarget]::Process)
            }
        }
    }
}

# Ensure log directory exists.
$localAppDataDir = $env:LOCALAPPDATA
if (-not $localAppDataDir) {
    $localAppDataDir = [Environment]::GetFolderPath([Environment+SpecialFolder]::LocalApplicationData)
}
$logDir = Join-Path $localAppDataDir "vaultchron\logs"
if (-not (Test-Path -LiteralPath $logDir)) {
    New-Item -ItemType Directory -Path $logDir -Force | Out-Null
}
$logFile = Join-Path $logDir "vaultchron.log"

$startTime = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
"=== vaultchron run started at $startTime ===" | Out-File -LiteralPath $logFile -Append -Encoding utf8

$prevLocation = Get-Location
Set-Location -LiteralPath $installDir

$prevEAP = $ErrorActionPreference
$ErrorActionPreference = "Continue"

$configFile = Join-Path $installDir "config.yaml"
$LASTEXITCODE = 0

# PowerShell 5.1 decodes native output with the OEM code page and wraps stderr
# in ErrorRecord objects. Ensure UTF-8 decoding and convert output to plain text strings.
$prevEnc = [Console]::OutputEncoding
try { [Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false) } catch { }

& $exePath -config $configFile 2>&1 | ForEach-Object { "$_" } | Out-File -LiteralPath $logFile -Append -Encoding utf8
$exitCode = $LASTEXITCODE

try { [Console]::OutputEncoding = $prevEnc } catch { }

$ErrorActionPreference = $prevEAP
Set-Location -LiteralPath $prevLocation

$finishTime = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
"=== vaultchron run finished at $finishTime with exit code $exitCode ===" | Out-File -LiteralPath $logFile -Append -Encoding utf8

exit $exitCode
