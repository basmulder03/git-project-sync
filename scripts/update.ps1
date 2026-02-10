# Git Project Sync - Update Script for Windows
# Usage: 
#   iwr -useb https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/update.ps1 | iex

[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"

# Configuration
$Repo = "basmulder03/git-project-sync"
$BinaryName = "mirror-cli"

# Functions
function Write-Info {
    param([string]$Message)
    Write-Host "==> $Message" -ForegroundColor Green
}

function Write-Warn {
    param([string]$Message)
    Write-Host "Warning: $Message" -ForegroundColor Yellow
}

function Write-ErrorMsg {
    param([string]$Message)
    Write-Host "Error: $Message" -ForegroundColor Red
}

function Get-CurrentVersion {
    try {
        if (Get-Command $BinaryName -ErrorAction SilentlyContinue) {
            $versionOutput = & $BinaryName --version 2>&1
            if ($versionOutput -match '(\d+\.\d+\.\d+)') {
                return $matches[1]
            }
        }
        return "not_installed"
    }
    catch {
        return "not_installed"
    }
}

function Get-LatestRelease {
    try {
        $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -UseBasicParsing
        return $release.tag_name
    }
    catch {
        return $null
    }
}

function Get-InstallLocation {
    try {
        if (Get-Command $BinaryName -ErrorAction SilentlyContinue) {
            $binaryPath = (Get-Command $BinaryName).Source
            return Split-Path $binaryPath -Parent
        }
        else {
            return "$env:LOCALAPPDATA\Programs\mirror-cli"
        }
    }
    catch {
        return "$env:LOCALAPPDATA\Programs\mirror-cli"
    }
}

function Get-Architecture {
    if ([Environment]::Is64BitOperatingSystem) {
        return "x86_64"
    }
    else {
        Write-ErrorMsg "32-bit Windows is not supported"
        exit 1
    }
}

function Download-And-Update {
    param(
        [string]$Version,
        [string]$Architecture,
        [string]$InstallDir
    )
    
    # Construct download URL
    $assetName = "$BinaryName-windows-$Architecture.exe"
    $downloadUrl = "https://github.com/$Repo/releases/download/$Version/$assetName"
    
    Write-Info "Downloading $BinaryName $Version..."
    
    # Create temporary file
    $tempFile = Join-Path $env:TEMP "$BinaryName-update.exe"
    
    try {
        # Download the binary
        Invoke-WebRequest -Uri $downloadUrl -OutFile $tempFile -UseBasicParsing
    }
    catch {
        Write-ErrorMsg "Failed to download binary from $downloadUrl"
        exit 1
    }
    
    # Update binary
    $targetPath = Join-Path $InstallDir "$BinaryName.exe"
    Write-Info "Updating $targetPath..."
    
    # Remove existing binary
    if (Test-Path $targetPath) {
        Remove-Item $targetPath -Force
    }
    
    Move-Item $tempFile $targetPath -Force
    
    Write-Info "Update successful!" -ForegroundColor Green
}

# Main execution
try {
    Write-Info "Git Project Sync Updater"
    Write-Host ""
    
    # Check if binary is installed
    $currentVersion = Get-CurrentVersion
    
    if ($currentVersion -eq "not_installed") {
        Write-ErrorMsg "$BinaryName is not installed"
        Write-ErrorMsg "Please install it first using:"
        Write-ErrorMsg "  iwr -useb https://raw.githubusercontent.com/$Repo/main/scripts/install.ps1 | iex"
        exit 1
    }
    
    Write-Info "Current version: $currentVersion"
    
    # Get latest release version
    Write-Info "Checking for updates..."
    $latestVersion = Get-LatestRelease
    
    if (-not $latestVersion) {
        Write-ErrorMsg "Failed to fetch latest release version"
        exit 1
    }
    
    # Remove 'v' prefix for comparison if present
    $latestVersionNum = $latestVersion -replace '^v', ''
    
    Write-Info "Latest version: $latestVersion"
    
    # Compare versions
    if ($currentVersion -eq $latestVersionNum) {
        Write-Info "Already up to date!"
        exit 0
    }
    
    # Detect architecture
    $arch = Get-Architecture
    
    # Get install location
    $installDir = Get-InstallLocation
    
    Write-Info "Updating from $currentVersion to $latestVersion..."
    
    # Download and update
    Download-And-Update -Version $latestVersion -Architecture $arch -InstallDir $installDir
    
    # Verify update
    $newVersion = Get-CurrentVersion
    Write-Info "Updated to version: $newVersion"
}
catch {
    Write-ErrorMsg "Update failed: $_"
    exit 1
}
