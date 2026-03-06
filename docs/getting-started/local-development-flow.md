# Local Development Flow

Use the local development wrappers to run `syncctl`, `syncd`, and `synctui` directly from source while isolating runtime state from your normal user installation.

These wrappers:

- create/use `.dev/local/config.dev.yaml` inside the repository,
- copy from your current user config (`~/.config/git-project-sync/config.yaml` on Linux, `%APPDATA%\git-project-sync\config.yaml` on Windows),
- keep state DB isolated at `.dev/local/state.dev.db`,
- run tools with `go run` so you can test local code changes immediately.

## Linux

```bash
./scripts/dev/local.sh syncctl doctor
./scripts/dev/local.sh syncctl sync all --dry-run
./scripts/dev/local.sh syncd --once
./scripts/dev/local.sh synctui
```

Force refresh from your current user config:

```bash
./scripts/dev/local.sh --refresh-config syncctl config show
```

## Windows (PowerShell)

```powershell
./scripts/dev/local.ps1 syncctl doctor
./scripts/dev/local.ps1 syncctl sync all --dry-run
./scripts/dev/local.ps1 syncd --once
./scripts/dev/local.ps1 synctui
```

Force refresh from your current user config:

```powershell
./scripts/dev/local.ps1 -RefreshConfig syncctl config show
```

## Optional override

Set `SYNCDEV_SOURCE_CONFIG` to copy from a different source config file.

Examples:

```bash
SYNCDEV_SOURCE_CONFIG=/tmp/custom-config.yaml ./scripts/dev/local.sh syncctl config show
```

```powershell
$env:SYNCDEV_SOURCE_CONFIG = "C:\temp\custom-config.yaml"
./scripts/dev/local.ps1 syncctl config show
```
