# Git Project Sync - Installation Script for Windows
# Usage: 
#   PowerShell (Admin): iwr -useb https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/install.ps1 | iex
#   Or: Invoke-WebRequest -Uri https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/install.ps1 -UseBasicParsing | Invoke-Expression

[CmdletBinding()]
param(
    [string]$InstallDir = "$env:LOCALAPPDATA\Programs\mirror-cli",
    [switch]$AddToPath = $false
)

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

function Get-LatestRelease {
    try {
        $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -UseBasicParsing
        return $release.tag_name
    }
    catch {
        return $null
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

function Download-And-Install {
    param(
        [string]$Version,
        [string]$Architecture
    )
    
    # Construct download URL
    $assetName = "$BinaryName-windows-$Architecture.exe"
    $downloadUrl = "https://github.com/$Repo/releases/download/$Version/$assetName"
    
    Write-Info "Downloading $BinaryName $Version for windows-$Architecture..."
    Write-Info "URL: $downloadUrl"
    
    # Create temporary file
    $tempFile = Join-Path $env:TEMP "$BinaryName.exe"
    
    try {
        # Download the binary
        Invoke-WebRequest -Uri $downloadUrl -OutFile $tempFile -UseBasicParsing
    }
    catch {
        Write-ErrorMsg "Failed to download binary from $downloadUrl"
        Write-ErrorMsg "Please check if the release exists and the asset is available"
        exit 1
    }
    
    # Create install directory if it doesn't exist
    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    }
    
    # Install binary
    $targetPath = Join-Path $InstallDir "$BinaryName.exe"
    Write-Info "Installing to $targetPath..."
    
    # Remove existing binary if present
    if (Test-Path $targetPath) {
        Remove-Item $targetPath -Force
    }
    
    Move-Item $tempFile $targetPath -Force
    
    Write-Info "Installation successful!" -ForegroundColor Green
}

function Add-ToPath {
    param([string]$Directory)
    
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    
    if ($userPath -split ';' -contains $Directory) {
        Write-Info "Directory already in PATH"
        return
    }
    
    Write-Info "Adding $Directory to user PATH..."
    
    $newPath = if ($userPath) { "$userPath;$Directory" } else { $Directory }
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    
    # Update current session PATH
    $env:Path = "$env:Path;$Directory"
    
    Write-Info "Added to PATH. You may need to restart your terminal for changes to take effect."
}

function Test-Installation {
    $binaryPath = Join-Path $InstallDir "$BinaryName.exe"
    
    if (Test-Path $binaryPath) {
        Write-Info "✓ $BinaryName is installed at $binaryPath"
        
        # Try to get version
        try {
            $version = & $binaryPath --version 2>&1
            Write-Info "Version: $version"
        }
        catch {
            Write-Warn "Unable to get version"
        }
    }
    else {
        Write-ErrorMsg "Installation verification failed"
        exit 1
    }
    
    # Check if in PATH
    if (Get-Command $BinaryName -ErrorAction SilentlyContinue) {
        Write-Info "✓ $BinaryName is available in your PATH"
    }
    else {
        Write-Warn "$BinaryName is not in your PATH"
        Write-Warn "Run with -AddToPath flag to add it automatically, or add manually:"
        Write-Warn "  `$env:Path += ';$InstallDir'"
        Write-Warn ""
        Write-Warn "Or run directly: $binaryPath"
    }
}

function Show-NextSteps {
    Write-Host ""
    Write-Info "Next steps:"
    Write-Host "  1. Initialize config: $BinaryName config init --root C:\path\to\mirrors"
    Write-Host "  2. Add a target: $BinaryName target add --provider github --scope your-org"
    Write-Host "  3. Set token: $BinaryName token set --provider github --scope your-org --token YOUR_TOKEN"
    Write-Host "  4. Run sync: $BinaryName sync"
    Write-Host ""
    Write-Host "For more information, visit: https://github.com/$Repo"
}

# Main execution
try {
    Write-Info "Git Project Sync Installer"
    Write-Host ""
    
    # Detect architecture
    $arch = Get-Architecture
    Write-Info "Detected: windows-$arch"
    
    # Get latest release version
    Write-Info "Fetching latest release..."
    $version = Get-LatestRelease
    
    if (-not $version) {
        Write-ErrorMsg "Failed to fetch latest release version"
        Write-ErrorMsg "Please check your internet connection and try again"
        exit 1
    }
    
    Write-Info "Latest version: $version"
    
    # Download and install
    Download-And-Install -Version $version -Architecture $arch
    
    # Add to PATH if requested
    if ($AddToPath) {
        Add-ToPath -Directory $InstallDir
    }
    
    # Verify installation
    Test-Installation
    
    # Show next steps
    Show-NextSteps
}
catch {
    Write-ErrorMsg "Installation failed: $_"
    exit 1
}
