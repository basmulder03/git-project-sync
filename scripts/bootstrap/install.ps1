param(
  [ValidateSet("user")]
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

$binDir = "$env:LOCALAPPDATA\git-project-sync\bin"
$configPath = "$env:APPDATA\git-project-sync\config.yaml"

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
$synctuiPath = Join-Path $binDir "synctui.exe"

Invoke-WebRequest -Uri "$baseUrl/syncd_windows_amd64.exe" -OutFile $syncdPath
Invoke-WebRequest -Uri "$baseUrl/syncctl_windows_amd64.exe" -OutFile $syncctlPath
Invoke-WebRequest -Uri "$baseUrl/synctui_windows_amd64.exe" -OutFile $synctuiPath

$env:BIN_PATH = $syncdPath
$env:CONFIG_PATH = $configPath

$installScriptPath = Join-Path $env:TEMP "gps-install.ps1"
$installScriptUri = "https://raw.githubusercontent.com/$Repo/main/scripts/install.ps1"

try {
  Invoke-WebRequest -Uri $installScriptUri -OutFile $installScriptPath
  & $installScriptPath -Mode $Mode
} finally {
  if (Test-Path -LiteralPath $installScriptPath) {
    Remove-Item -LiteralPath $installScriptPath -Force
  }
}

Write-Host "Bootstrap install complete"
Write-Host "syncd: $syncdPath"
Write-Host "syncctl: $syncctlPath"
Write-Host "synctui: $synctuiPath"
Write-Host "config: $configPath"
Write-Host ""
Write-Host "Next steps:"
Write-Host "1) Validate install: $syncctlPath --version"
Write-Host "2) Add a source: $syncctlPath source add github <source-id> --account <account>"
Write-Host "3) Login PAT: $syncctlPath auth login <source-id> --token <pat>"
Write-Host "4) Register repos: $syncctlPath repo add <path> --source-id <source-id>"
Write-Host "5) Dry-run first sync: $syncctlPath sync all --dry-run"
Write-Host "6) Monitor health: $syncctlPath doctor ; $syncctlPath daemon status"
Write-Host ""
Write-Host "See docs/getting-started/first-run-onboarding.md for guided onboarding."
