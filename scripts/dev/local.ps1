param(
  [switch]$RefreshConfig,
  [ValidateSet("syncctl", "syncd", "synctui")]
  [Parameter(Mandatory = $true)]
  [string]$Tool,
  [Parameter(ValueFromRemainingArguments = $true)]
  [string[]]$Args
)

$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = (Resolve-Path (Join-Path $scriptDir "../..")).Path

$devDir = Join-Path $repoRoot ".dev/local"
$devConfig = Join-Path $devDir "config.dev.yaml"
$devStateDb = Join-Path $devDir "state.dev.db"

if ($env:SYNCDEV_SOURCE_CONFIG) {
  $sourceConfig = $env:SYNCDEV_SOURCE_CONFIG
} else {
  $sourceConfig = Join-Path $env:APPDATA "git-project-sync/config.yaml"
}

New-Item -ItemType Directory -Path $devDir -Force | Out-Null

$shouldCopy = $false
if (Test-Path -LiteralPath $sourceConfig -PathType Leaf) {
  if ($RefreshConfig -or -not (Test-Path -LiteralPath $devConfig -PathType Leaf)) {
    $shouldCopy = $true
  } else {
    $sourceTime = (Get-Item -LiteralPath $sourceConfig).LastWriteTimeUtc
    $devTime = (Get-Item -LiteralPath $devConfig).LastWriteTimeUtc
    if ($sourceTime -gt $devTime) {
      $shouldCopy = $true
    }
  }

  if ($shouldCopy) {
    Copy-Item -LiteralPath $sourceConfig -Destination $devConfig -Force
  }
} elseif (-not (Test-Path -LiteralPath $devConfig -PathType Leaf)) {
  Copy-Item -LiteralPath (Join-Path $repoRoot "configs/config.example.yaml") -Destination $devConfig -Force
}

Push-Location $repoRoot
try {
  go run ./cmd/syncctl --config $devConfig config set state.db_path $devStateDb | Out-Null

  Write-Host "syncdev config: $devConfig"
  Write-Host "syncdev state:  $devStateDb"

  switch ($Tool) {
    "syncctl" { go run ./cmd/syncctl --config $devConfig @Args }
    "syncd" { go run ./cmd/syncd --config $devConfig @Args }
    "synctui" { go run ./cmd/synctui --config $devConfig @Args }
  }

  if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
  }
} finally {
  Pop-Location
}
