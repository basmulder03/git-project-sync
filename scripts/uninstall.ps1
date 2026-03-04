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

try {
  schtasks /Delete /F /TN $taskName | Out-Null
} catch {
  Write-Host "Task '$taskName' not found or already removed"
}

Write-Host "Uninstalled scheduled task '$taskName' in $Mode mode"
