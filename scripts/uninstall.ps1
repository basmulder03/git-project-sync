param(
  [ValidateSet("user", "system")]
  [string]$Mode = "user"
)

$ErrorActionPreference = "Stop"

# Require Administrator – sc.exe delete requires elevation.
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
  throw "Uninstalling a Windows Service requires Administrator privileges. Re-run this script in an elevated shell."
}

$serviceName = "GitProjectSync"

Write-Host "Stopping service '$serviceName' (if running)..."
& sc.exe stop $serviceName 2>&1 | Out-Null
# Give the service a moment to stop before deleting it.
Start-Sleep -Seconds 2

$result = & sc.exe delete $serviceName 2>&1
if ($LASTEXITCODE -ne 0) {
  # 1060 = ERROR_SERVICE_DOES_NOT_EXIST – treat as already uninstalled.
  if ($result -match "1060") {
    Write-Host "Service '$serviceName' was not found (already removed)."
  } else {
    throw "sc.exe delete $serviceName failed (exit $LASTEXITCODE): $result"
  }
} else {
  Write-Host "Uninstalled Windows Service '$serviceName' in $Mode mode"
}
