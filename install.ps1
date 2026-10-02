# VaultChron installer for Windows.
#
# Running via 'irm <url> | iex' executes in-memory as commands in the current
# console session, working under any execution policy without saving a script
# file. Do not save this to a .ps1 file and run it if script execution is
# restricted by ExecutionPolicy.
#
# Environment variables:
#   VAULTCHRON_VERSION      Target release version (e.g. v0.1.0). If unset,
#                           the latest published release is detected.
#   VAULTCHRON_INSTALL_DIR  Target directory for installed binaries.
#                           Defaults to $env:LOCALAPPDATA\Programs\vaultchron.
#   VAULTCHRON_BASE_URL     Base URL for release downloads.
#                           Defaults to https://github.com/ZeezyCodes/vaultchron/releases/download/<version>.

& {
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue'

    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    $isAmd64 = ($env:PROCESSOR_ARCHITECTURE -eq 'AMD64') -or ($env:PROCESSOR_ARCHITEW6432 -eq 'AMD64')
    if (-not $isAmd64) {
        throw "VaultChron on Windows currently supports only AMD64 (x86_64). For other architectures, visit https://github.com/ZeezyCodes/vaultchron/releases"
    }

    $version = $env:VAULTCHRON_VERSION
    if ($version) {
        if ($version -notmatch '^v\d+\.\d+\.\d+') {
            throw "Invalid version format '$version'. Expected format like v1.2.3"
        }
    } else {
        try {
            $release = Invoke-RestMethod -Uri 'https://api.github.com/repos/ZeezyCodes/vaultchron/releases/latest' -Headers @{ 'User-Agent' = 'vaultchron-installer' }
            $version = $release.tag_name
            if ((-not $version) -or ($version -notmatch '^v\d+\.\d+\.\d+')) {
                throw "Invalid release tag"
            }
        } catch {
            throw "Could not determine the latest release (possibly rate-limited); set VAULTCHRON_VERSION"
        }
    }

    $baseUrl = $env:VAULTCHRON_BASE_URL
    if (-not $baseUrl) {
        $baseUrl = "https://github.com/ZeezyCodes/vaultchron/releases/download/$version"
    }

    $verNum = $version.TrimStart('v')
    $archiveName = "vaultchron_${verNum}_windows_amd64.zip"
    $checksumsName = "checksums.txt"

    $guid = [System.Guid]::NewGuid().ToString()
    $tempDir = Join-Path -Path $env:TEMP -ChildPath "vaultchron-$guid"
    New-Item -ItemType Directory -Path $tempDir -Force | Out-Null

    try {
        $archivePath = Join-Path -Path $tempDir -ChildPath $archiveName
        $checksumsPath = Join-Path -Path $tempDir -ChildPath $checksumsName

        Invoke-WebRequest -Uri "$baseUrl/$archiveName" -OutFile $archivePath -UseBasicParsing
        Invoke-WebRequest -Uri "$baseUrl/$checksumsName" -OutFile $checksumsPath -UseBasicParsing

        $checksumContent = Get-Content -Path $checksumsPath
        $expectedHash = $null
        foreach ($line in $checksumContent) {
            $parts = $line.Trim() -split '\s+'
            if ($parts.Count -ge 2 -and $parts[1] -eq $archiveName) {
                $expectedHash = $parts[0]
                break
            }
        }

        if (-not $expectedHash) {
            throw "No checksum found for $archiveName in $checksumsName"
        }

        $actualHash = (Get-FileHash -Path $archivePath -Algorithm SHA256).Hash
        if ($actualHash.ToLower() -ne $expectedHash.ToLower()) {
            throw "Checksum mismatch for $archiveName`n  expected: $expectedHash`n  actual:   $actualHash"
        }

        $extractDir = Join-Path -Path $tempDir -ChildPath "extracted"
        Expand-Archive -Path $archivePath -DestinationPath $extractDir -Force

        $installDir = $env:VAULTCHRON_INSTALL_DIR
        if (-not $installDir) {
            $installDir = Join-Path -Path $env:LOCALAPPDATA -ChildPath "Programs\vaultchron"
        }

        if (-not (Test-Path -Path $installDir)) {
            New-Item -ItemType Directory -Path $installDir -Force | Out-Null
        }

        $srcVaultchron = Join-Path -Path $extractDir -ChildPath "vaultchron.exe"
        $srcMigrate = Join-Path -Path $extractDir -ChildPath "vaultchron_migrate.exe"

        if ((-not (Test-Path -Path $srcVaultchron)) -or (-not (Test-Path -Path $srcMigrate))) {
            throw "Archive did not contain expected executables"
        }

        Copy-Item -Path $srcVaultchron -Destination (Join-Path -Path $installDir -ChildPath "vaultchron.exe") -Force
        Copy-Item -Path $srcMigrate -Destination (Join-Path -Path $installDir -ChildPath "vaultchron_migrate.exe") -Force

        $normInstallDir = $installDir.TrimEnd('\')
        $currentUserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
        $pathModified = $false

        $pathEntries = @()
        if ($currentUserPath) {
            $pathEntries = $currentUserPath -split ';'
        }

        $alreadyInPath = $false
        foreach ($entry in $pathEntries) {
            if ($entry.TrimEnd('\').Equals($normInstallDir, [System.StringComparison]::OrdinalIgnoreCase)) {
                $alreadyInPath = $true
                break
            }
        }

        if (-not $alreadyInPath) {
            if ($currentUserPath) {
                $newUserPath = "$currentUserPath;$normInstallDir"
            } else {
                $newUserPath = $normInstallDir
            }
            [Environment]::SetEnvironmentVariable('Path', $newUserPath, 'User')
            $pathModified = $true
        }

        $sessionEntries = $env:Path -split ';'
        $inSessionPath = $false
        foreach ($entry in $sessionEntries) {
            if ($entry.TrimEnd('\').Equals($normInstallDir, [System.StringComparison]::OrdinalIgnoreCase)) {
                $inSessionPath = $true
                break
            }
        }

        if (-not $inSessionPath) {
            if ($env:Path) {
                $env:Path = "$env:Path;$normInstallDir"
            } else {
                $env:Path = $normInstallDir
            }
        }

        Write-Host "Installed vaultchron.exe and vaultchron_migrate.exe to $installDir"
        $installedExe = Join-Path -Path $installDir -ChildPath "vaultchron.exe"
        & "$installedExe" -version

        if ($pathModified) {
            Write-Host "Open a new terminal for the PATH change to apply."
        }
    } finally {
        if (Test-Path -Path $tempDir) {
            Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}
