param(
  [ValidateSet("user", "system")]
  [string]$Mode = "user",
  [string]$Version = "latest",
  [string]$Repo = "basmulder03/git-project-sync"
)

$ErrorActionPreference = "Stop"

if ($env:OS -ne "Windows_NT") {
  throw "This bootstrap installer supports Windows only"
}

if ($env:PROCESSOR_ARCHITECTURE -notin @("AMD64", "x86_64")) {
  throw "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE"
}

if ($Mode -eq "system") {
  $isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
  if (-not $isAdmin) {
    throw "System bootstrap install requires Administrator privileges"
  }
}

if ($Mode -eq "system") {
  $binDir = "$env:ProgramFiles\git-project-sync"
  $configPath = "$env:ProgramData\git-project-sync\config.yaml"
} else {
  $binDir = "$env:LOCALAPPDATA\git-project-sync\bin"
  $configPath = "$env:APPDATA\git-project-sync\config.yaml"
}

if (-not (Test-Path -LiteralPath $binDir)) {
  New-Item -ItemType Directory -Path $binDir -Force | Out-Null
}

if ($Version -eq "latest") {
  $baseUrl = "https://github.com/$Repo/releases/latest/download"
} else {
  $baseUrl = "https://github.com/$Repo/releases/download/$Version"
}

$syncdPath = Join-Path $binDir "syncd.exe"
$syncctlPath = Join-Path $binDir "syncctl.exe"

Invoke-WebRequest -Uri "$baseUrl/syncd_windows_amd64.exe" -OutFile $syncdPath
Invoke-WebRequest -Uri "$baseUrl/syncctl_windows_amd64.exe" -OutFile $syncctlPath

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = Resolve-Path (Join-Path $scriptDir "..\..")

$env:BIN_PATH = $syncdPath
$env:CONFIG_PATH = $configPath

& (Join-Path $repoRoot "scripts\install.ps1") -Mode $Mode

Write-Host "Bootstrap install complete"
Write-Host "syncd: $syncdPath"
Write-Host "syncctl: $syncctlPath"
