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
$binPath = if ($env:BIN_PATH) { $env:BIN_PATH } else { "$env:ProgramFiles\git-project-sync\syncd.exe" }
$configPath = if ($env:CONFIG_PATH) { $env:CONFIG_PATH } else { "$env:APPDATA\git-project-sync\config.yaml" }
$exec = "`"$binPath`" --config `"$configPath`""

if ($Mode -eq "system") {
  schtasks /Create /F /SC MINUTE /MO 5 /TN $taskName /TR $exec /RL HIGHEST /RU SYSTEM | Out-Null
} else {
  schtasks /Create /F /SC MINUTE /MO 5 /TN $taskName /TR $exec /RL LIMITED | Out-Null
}

schtasks /Query /TN $taskName | Out-Null
Write-Host "Installed scheduled task '$taskName' in $Mode mode"
