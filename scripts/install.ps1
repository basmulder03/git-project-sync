param(
  [ValidateSet("user", "system")]
  [string]$Mode = "user"
)

$ErrorActionPreference = "Stop"

# Require Administrator – sc.exe create requires elevation regardless of mode.
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
  throw "Installing a Windows Service requires Administrator privileges. Re-run this script in an elevated shell."
}

function Invoke-Sc {
  param([string[]]$Arguments)
  $result = & sc.exe @Arguments 2>&1
  if ($LASTEXITCODE -ne 0) {
    throw "sc.exe $($Arguments -join ' ') failed (exit $LASTEXITCODE): $result"
  }
  return $result
}

function Add-ToPath {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Directory,
    [ValidateSet("User", "Machine")]
    [string]$Scope = "User"
  )

  if (-not (Test-Path -LiteralPath $Directory -PathType Container)) {
    return
  }

  $current = [Environment]::GetEnvironmentVariable("Path", $Scope)
  $segments = @()
  if ($current) {
    $segments = $current.Split(';') | Where-Object { $_ -and $_.Trim() -ne "" }
  }

  $alreadyPresent = $false
  foreach ($segment in $segments) {
    if ($segment.TrimEnd('\') -ieq $Directory.TrimEnd('\')) {
      $alreadyPresent = $true
      break
    }
  }

  if (-not $alreadyPresent) {
    $newValue = (($segments + $Directory) | Select-Object -Unique) -join ';'
    [Environment]::SetEnvironmentVariable("Path", $newValue, $Scope)
  }

  if (-not (($env:Path -split ';') | Where-Object { $_.TrimEnd('\') -ieq $Directory.TrimEnd('\') })) {
    if ($env:Path -and $env:Path.Trim() -ne "") {
      $env:Path = "$env:Path;$Directory"
    } else {
      $env:Path = $Directory
    }
  }
}

$serviceName = "GitProjectSync"

# Migrate any legacy Task Scheduler job from the pre-service era.
$legacyTask = & schtasks /Query /TN $serviceName 2>&1
if ($LASTEXITCODE -eq 0) {
  Write-Host "Migrating legacy Task Scheduler job '$serviceName' to Windows Service..."
  & schtasks /Delete /F /TN $serviceName 2>&1 | Out-Null
}

if ($env:BIN_PATH) {
  $binPath = $env:BIN_PATH
} elseif ($Mode -eq "system") {
  $binPath = "$env:ProgramFiles\git-project-sync\bin\syncd.exe"
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

# Remove any pre-existing service entry (idempotent re-install).
$existing = & sc.exe query $serviceName 2>&1
if ($LASTEXITCODE -eq 0) {
  Write-Host "Removing existing service '$serviceName'..."
  & sc.exe stop $serviceName 2>&1 | Out-Null
  Start-Sleep -Seconds 2
  Invoke-Sc @("delete", $serviceName) | Out-Null
  # Give SCM a moment to clean up.
  Start-Sleep -Seconds 2
}

$binCmd = "`"$binPath`" --config `"$configPath`""

$startType = if ($Mode -eq "system") { "auto" } else { "demand" }
$runAs     = if ($Mode -eq "system") { "LocalService" } else { $null }

$createArgs = @(
  "create", $serviceName,
  "binPath=", $binCmd,
  "start=", $startType,
  "DisplayName=", "Git Project Sync"
)
if ($runAs) {
  $createArgs += @("obj=", $runAs)
}

Invoke-Sc $createArgs | Out-Null
Invoke-Sc @("description", $serviceName, "Keeps local git repositories in sync with their remote default branch") | Out-Null

# Start the service immediately.
Invoke-Sc @("start", $serviceName) | Out-Null

$pathScope = if ($Mode -eq "system") { "Machine" } else { "User" }
Add-ToPath -Directory (Split-Path -Parent $binPath) -Scope $pathScope

Write-Host "Installed Windows Service '$serviceName' in $Mode mode"
Write-Host "Binary:  $binPath"
Write-Host "Config:  $configPath"
Write-Host "Added $(Split-Path -Parent $binPath) to $pathScope PATH scope"
