param(
  [ValidateSet("user", "system")]
  [string]$Mode = "user"
)

$ErrorActionPreference = "Stop"

if ($Mode -eq "system") {
  $isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
  if (-not $isAdmin) {
    throw "System install requires Administrator privileges"
  }
}

$taskName = "GitProjectSync"
if ($env:BIN_PATH) {
  $binPath = $env:BIN_PATH
} elseif ($Mode -eq "system") {
  $binPath = "$env:ProgramFiles\git-project-sync\syncd.exe"
} else {
  $binPath = "$env:LOCALAPPDATA\git-project-sync\bin\syncd.exe"
}

if ($env:CONFIG_PATH) {
  $configPath = $env:CONFIG_PATH
} elseif ($Mode -eq "system") {
  $configPath = "$env:ProgramData\git-project-sync\config.yaml"
} else {
  $configPath = "$env:APPDATA\git-project-sync\config.yaml"
}

if (-not (Test-Path -LiteralPath $binPath -PathType Leaf)) {
  throw "syncd binary not found at $binPath"
}

$configDir = Split-Path -Parent $configPath
if (-not (Test-Path -LiteralPath $configDir)) {
  New-Item -ItemType Directory -Path $configDir -Force | Out-Null
}

if (-not (Test-Path -LiteralPath $configPath -PathType Leaf)) {
  @"
daemon:
  interval: 5m
repositories: []
sources: []
"@ | Set-Content -Path $configPath -NoNewline
}

$exec = "`"$binPath`" --config `"$configPath`""

if ($Mode -eq "system") {
  schtasks /Create /F /SC MINUTE /MO 5 /TN $taskName /TR $exec /RL HIGHEST /RU SYSTEM | Out-Null
} else {
  schtasks /Create /F /SC MINUTE /MO 5 /TN $taskName /TR $exec /RL LIMITED | Out-Null
}

schtasks /Query /TN $taskName | Out-Null
Write-Host "Installed scheduled task '$taskName' in $Mode mode"
