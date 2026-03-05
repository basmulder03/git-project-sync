param(
  [ValidateSet("user", "system")]
  [string]$Mode = "user"
)

$ErrorActionPreference = "Stop"

function Invoke-Schtasks {
  param(
    [Parameter(Mandatory = $true)]
    [string[]]$Arguments
  )

  & schtasks.exe @Arguments
  if ($LASTEXITCODE -ne 0) {
    throw "schtasks failed with exit code $LASTEXITCODE: $($Arguments -join ' ')"
  }
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
    if ($segment.TrimEnd('\\') -ieq $Directory.TrimEnd('\\')) {
      $alreadyPresent = $true
      break
    }
  }

  if (-not $alreadyPresent) {
    $newValue = (($segments + $Directory) | Select-Object -Unique) -join ';'
    [Environment]::SetEnvironmentVariable("Path", $newValue, $Scope)
  }

  if (-not (($env:Path -split ';') | Where-Object { $_.TrimEnd('\\') -ieq $Directory.TrimEnd('\\') })) {
    if ($env:Path -and $env:Path.Trim() -ne "") {
      $env:Path = "$env:Path;$Directory"
    } else {
      $env:Path = $Directory
    }
  }
}

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

$taskCommand = "cmd /c `"`"$binPath`" --config `"$configPath`"`""

$pathScope = if ($Mode -eq "system") { "Machine" } else { "User" }
Add-ToPath -Directory (Split-Path -Parent $binPath) -Scope $pathScope

if ($Mode -eq "system") {
  Invoke-Schtasks -Arguments @("/Create", "/F", "/SC", "MINUTE", "/MO", "5", "/TN", $taskName, "/TR", $taskCommand, "/RL", "HIGHEST", "/RU", "SYSTEM")
} else {
  Invoke-Schtasks -Arguments @("/Create", "/F", "/SC", "MINUTE", "/MO", "5", "/TN", $taskName, "/TR", $taskCommand, "/RL", "LIMITED")
}

Invoke-Schtasks -Arguments @("/Query", "/TN", $taskName)
Write-Host "Installed scheduled task '$taskName' in $Mode mode"
Write-Host "Added $(Split-Path -Parent $binPath) to $pathScope PATH scope"
