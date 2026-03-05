param(
  [ValidateSet("user", "system")]
  [string]$Mode = "user"
)

$ErrorActionPreference = "Stop"

if ($Mode -eq "system") {
  $isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
  if (-not $isAdmin) {
    throw "System uninstall requires Administrator privileges"
  }
}

$taskName = "GitProjectSync"

if ($env:CONFIG_PATH) {
  $configPath = $env:CONFIG_PATH
} elseif ($Mode -eq "system") {
  $configPath = "$env:ProgramData\git-project-sync\config.yaml"
} else {
  $configPath = "$env:APPDATA\git-project-sync\config.yaml"
}

try {
  schtasks /Delete /F /TN $taskName | Out-Null
} catch {
  Write-Host "Task '$taskName' not found or already removed"
}

$launcherPath = Join-Path (Split-Path -Parent $configPath) "run-syncd.cmd"
if (Test-Path -LiteralPath $launcherPath -PathType Leaf) {
  Remove-Item -LiteralPath $launcherPath -Force
}

Write-Host "Uninstalled scheduled task '$taskName' in $Mode mode"
