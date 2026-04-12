param(
    [string]$Version = "latest",
    [string]$InstallDir = "",
    [switch]$NoPath
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

try {
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
} catch {
}

$Repo = "php-workx/fuse"
$ArchiveTemplate = "fuse_{{ARCHIVE_OS}}_{{ARCHIVE_ARCH}}.zip"

function Get-FuseArchitecture {
    $arch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } elseif ($env:PROCESSOR_ARCHITECTURE) { $env:PROCESSOR_ARCHITECTURE } else { "" }
    $arch = $arch.ToLowerInvariant()

    switch ($arch) {
        "amd64" { return "amd64" }
        "x86_64" { return "amd64" }
        "arm64" { return "arm64" }
        "aarch64" { return "arm64" }
        default {
            if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq [System.Runtime.InteropServices.Architecture]::Arm64) {
                return "arm64"
            }
            if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq [System.Runtime.InteropServices.Architecture]::X64) {
                return "amd64"
            }
            throw "Unsupported Windows architecture: $arch"
        }
    }
}

function Add-FuseToUserPath {
    param([string]$BinDir)

    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $segments = @()
    if ($userPath) {
        $segments = @($userPath -split ";" | Where-Object { $_ -ne "" })
    }

    $normalizedBin = $BinDir.TrimEnd("\")
    foreach ($segment in $segments) {
        if ($segment.TrimEnd("\").Equals($normalizedBin, [StringComparison]::OrdinalIgnoreCase)) {
            return $false
        }
    }

    $newUserPath = if ($segments.Count -gt 0) {
        ($segments + $BinDir) -join ";"
    } else {
        $BinDir
    }

    [Environment]::SetEnvironmentVariable("Path", $newUserPath, "User")

    $processSegments = @()
    if ($env:Path) {
        $processSegments = @($env:Path -split ";" | Where-Object { $_ -ne "" })
    }
    $env:Path = ($processSegments + $BinDir) -join ";"
    return $true
}

function Test-FuseWindowsHost {
    if ($PSVersionTable.PSEdition -eq "Desktop") {
        return $true
    }

    return [System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform(
        [System.Runtime.InteropServices.OSPlatform]::Windows
    )
}

if (-not (Test-FuseWindowsHost)) {
    throw "install.ps1 is only supported on Windows."
}

$arch = Get-FuseArchitecture
$archiveName = $ArchiveTemplate.Replace("{{ARCHIVE_OS}}", "windows").Replace("{{ARCHIVE_ARCH}}", $arch)
$releaseBase = if ($Version -eq "latest") {
    "https://github.com/$Repo/releases/latest/download"
} else {
    "https://github.com/$Repo/releases/download/$Version"
}

if (-not $InstallDir) {
    $InstallDir = Join-Path $env:LOCALAPPDATA "Programs\fuse"
}
$binDir = Join-Path $InstallDir "bin"
$fuseExe = Join-Path $binDir "fuse.exe"

$tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("fuse-install-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tempDir | Out-Null

try {
    $archivePath = Join-Path $tempDir $archiveName
    $checksumsPath = Join-Path $tempDir "checksums.txt"
    $extractDir = Join-Path $tempDir "extract"

    $archiveUrl = "$releaseBase/$archiveName"
    $checksumsUrl = "$releaseBase/checksums.txt"

    Write-Host "Downloading $archiveUrl"
    Invoke-WebRequest -Uri $archiveUrl -OutFile $archivePath
    Invoke-WebRequest -Uri $checksumsUrl -OutFile $checksumsPath

    $expectedHash = $null
    foreach ($line in Get-Content $checksumsPath) {
        $parts = $line -split "\s+"
        if ($parts.Count -ge 2 -and $parts[-1] -eq $archiveName) {
            $expectedHash = $parts[0].ToUpperInvariant()
            break
        }
    }
    if (-not $expectedHash) {
        throw "No checksum found for $archiveName in checksums.txt"
    }

    $actualHash = (Get-FileHash -Algorithm SHA256 $archivePath).Hash.ToUpperInvariant()
    if ($actualHash -ne $expectedHash) {
        throw "Checksum mismatch for $archiveName. Expected $expectedHash, got $actualHash"
    }

    New-Item -ItemType Directory -Path $extractDir | Out-Null
    Expand-Archive -Path $archivePath -DestinationPath $extractDir -Force

    $extractedExe = Get-ChildItem -Path $extractDir -Recurse -Filter "fuse.exe" | Select-Object -First 1
    if (-not $extractedExe) {
        throw "Archive did not contain fuse.exe"
    }

    New-Item -ItemType Directory -Path $binDir -Force | Out-Null
    Copy-Item -Path $extractedExe.FullName -Destination $fuseExe -Force

    $pathChanged = $false
    if (-not $NoPath) {
        $pathChanged = Add-FuseToUserPath -BinDir $binDir
    }

    Write-Host "Installed fuse to $fuseExe"
    if ($pathChanged) {
        Write-Host "Added $binDir to your user PATH. Restart your terminal if 'fuse' is not found in other sessions."
    } elseif (-not $NoPath) {
        Write-Host "$binDir is already on your user PATH."
    }

    Write-Host ""
    Write-Host "Next:"
    Write-Host "  fuse --help"
    Write-Host "  fuse install claude"
    Write-Host "  fuse doctor --security"
} finally {
    if (Test-Path $tempDir) {
        Remove-Item -Path $tempDir -Recurse -Force
    }
}
