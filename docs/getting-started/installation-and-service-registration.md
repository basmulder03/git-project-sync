# Install and Service Registration

This guide covers bootstrap install plus service registration for Linux and Windows.

For first-run onboarding after install, follow `getting-started/first-run-onboarding.md`.

## Mode Comparison

- `user` mode:
  - no elevation required,
  - best for per-user repositories and credentials.
- `system` mode:
  - requires root/Administrator privileges,
  - suitable for machine-wide scheduled operation.

## Install Directly from GitHub `main`

Use these one-liners to execute the latest install scripts from the `main` branch.

### Bootstrap install (fresh machine)

::: code-group
```bash [Linux user]
curl -fsSL https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/bootstrap/install.sh | bash -s -- --user
```

```bash [Linux system]
curl -fsSL https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/bootstrap/install.sh | sudo bash -s -- --system
```

```powershell [Windows user]
powershell -NoProfile -ExecutionPolicy Bypass -Command "$p=Join-Path $env:TEMP 'gps-bootstrap-install.ps1'; iwr 'https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/bootstrap/install.ps1' -OutFile $p; & $p -Mode user; Remove-Item $p -Force"
```

```powershell [Windows system]
powershell -NoProfile -ExecutionPolicy Bypass -Command "$p=Join-Path $env:TEMP 'gps-bootstrap-install.ps1'; iwr 'https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/bootstrap/install.ps1' -OutFile $p; & $p -Mode system; Remove-Item $p -Force"
```
:::

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

```bash [Linux system]
sudo ./scripts/bootstrap/install.sh --system
```

```powershell [Windows user]
./scripts/bootstrap/install.ps1 -Mode user
```

```powershell [Windows system]
./scripts/bootstrap/install.ps1 -Mode system
```
:::

### Service registration only

::: code-group
```bash [Linux user]
./scripts/install.sh --user
```

```bash [Linux system]
sudo ./scripts/install.sh --system
```

```powershell [Windows user]
./scripts/install.ps1 -Mode user
```

```powershell [Windows system]
./scripts/install.ps1 -Mode system
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
  - `BIN_PATH` default: `~/.local/bin/syncd` (user), `/usr/local/bin/syncd` (system)
  - `CONFIG_PATH` default: `~/.config/git-project-sync/config.yaml` (user), `/etc/git-project-sync/config.yaml` (system)
- Windows:
  - `BIN_PATH` default: `%LOCALAPPDATA%\git-project-sync\bin\syncd.exe` (user), `%ProgramFiles%\git-project-sync\syncd.exe` (system)
  - `CONFIG_PATH` default: `%APPDATA%\git-project-sync\config.yaml` (user), `%ProgramData%\git-project-sync\config.yaml` (system)
  - installer adds the binary directory to PATH for the selected scope (`User` for user mode, `Machine` for system mode)

Service files:

- Linux user mode: `~/.config/systemd/user/git-project-sync.service`
- Linux system mode: `/etc/systemd/system/git-project-sync.service`
- Windows mode: Task Scheduler task `GitProjectSync`

## Offline/manual fallback

If bootstrap download is not possible, place `syncd` (and optionally `syncctl`) manually on disk, then run `scripts/install.sh` or `scripts/install.ps1` with `BIN_PATH` and `CONFIG_PATH` overrides.

## Notes

- Install/uninstall flows are designed to be idempotent.
- System mode checks for root/administrator privileges and fails fast when insufficient.
- Windows flow uses Task Scheduler and validates registration after install.
