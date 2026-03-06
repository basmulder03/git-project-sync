# Install and Service Registration

This guide covers bootstrap install plus service registration for Linux and Windows.

For first-run onboarding after install, follow `getting-started/first-run-onboarding.md`.

## Mode Comparison

- `user` mode only:
  - no elevation required,
  - best for per-user repositories and credentials.

## Install Directly from GitHub `main`

Use these one-liners to execute the latest install scripts from the `main` branch.

### Bootstrap install (fresh machine)

::: code-group
```bash [Linux user]
curl -fsSL https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/bootstrap/install.sh | bash -s -- --user
```

```powershell [Windows user]
powershell -NoProfile -ExecutionPolicy Bypass -Command "$p=Join-Path $env:TEMP 'gps-bootstrap-install.ps1'; iwr 'https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/bootstrap/install.ps1' -OutFile $p; & $p -Mode user; Remove-Item $p -Force"
```
:::

Bootstrap installs these binaries:

- `syncd` (daemon service target)
- `syncctl` (CLI)
- `synctui` (runtime dashboard)

Only `syncd` is registered with systemd/Task Scheduler. `synctui` remains an on-demand interactive tool.

### Uninstall (from GitHub `main`)

::: code-group
```bash [Linux user]
curl -fsSL https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/uninstall.sh | bash -s -- --user
```

```bash [Linux system]
curl -fsSL https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/uninstall.sh | sudo bash -s -- --system
```

```powershell [Windows user]
powershell -NoProfile -ExecutionPolicy Bypass -Command "$p=Join-Path $env:TEMP 'gps-uninstall.ps1'; iwr 'https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/uninstall.ps1' -OutFile $p; & $p -Mode user; Remove-Item $p -Force"
```

```powershell [Windows system]
powershell -NoProfile -ExecutionPolicy Bypass -Command "$p=Join-Path $env:TEMP 'gps-uninstall.ps1'; iwr 'https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/uninstall.ps1' -OutFile $p; & $p -Mode system; Remove-Item $p -Force"
```
:::

## Local Repository Script Usage

### Bootstrap install (fresh machine, no preinstalled binaries)

::: code-group
```bash [Linux user]
./scripts/bootstrap/install.sh --user
```

```powershell [Windows user]
./scripts/bootstrap/install.ps1 -Mode user
```
:::

### Service registration only

::: code-group
```bash [Linux user]
./scripts/install.sh --user
```

```powershell [Windows user]
./scripts/install.ps1 -Mode user
```
:::

### Uninstall (local scripts)

::: code-group
```bash [Linux user]
./scripts/uninstall.sh --user
```

```bash [Linux system]
sudo ./scripts/uninstall.sh --system
```

```powershell [Windows user]
./scripts/uninstall.ps1 -Mode user
```

```powershell [Windows system]
./scripts/uninstall.ps1 -Mode system
```
:::

Optional bootstrap parameters (both OS script families):

- pin version: `--version` (Linux), `-Version` (Windows)
- use fork repository: `--repo` (Linux), `-Repo` (Windows)

Environment overrides:

- Linux:
  - `BIN_PATH` default: `~/.local/bin/syncd`
  - `CONFIG_PATH` default: `~/.config/git-project-sync/config.yaml`
- Windows:
  - `BIN_PATH` default: `%LOCALAPPDATA%\git-project-sync\bin\syncd.exe`
  - `CONFIG_PATH` default: `%APPDATA%\git-project-sync\config.yaml`
  - installer adds the binary directory to PATH in `User` scope

Service files:

- Linux user mode: `~/.config/systemd/user/git-project-sync.service`
- Windows mode: Task Scheduler task `GitProjectSync`

## Offline/manual fallback

If bootstrap download is not possible, place `syncd` (and optionally `syncctl`) manually on disk, then run `scripts/install.sh` or `scripts/install.ps1` with `BIN_PATH` and `CONFIG_PATH` overrides.

## Notes

- Install/uninstall flows are designed to be idempotent.
- Windows flow uses Task Scheduler and validates registration after install.
