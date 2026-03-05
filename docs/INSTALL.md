# Install and Service Registration

This guide covers bootstrap install plus service registration for Linux and Windows.

## Mode Comparison

- `user` mode:
  - No elevation required.
  - Best for per-user repositories and credentials.
- `system` mode:
  - Requires root/Administrator privileges.
  - Suitable for machine-wide scheduled operation.

## Linux

### TUI installer (recommended)

`syncsetup` provides a small interactive installer TUI with the same visual style as `synctui`.

Launch:

```bash
syncsetup
```

Key actions:

- `m` toggle mode (`user`/`system`)
- `b` bootstrap + install/update (downloads and updates when target is newer)
- `r` repair/reinstall (forces binary re-download + registration)
- `i` install/register only
- `u` uninstall/unregister only
- `y` confirm a pending downgrade

Version behavior:

- Detects existing installation and compares installed `syncd --version` with target release.
- Automatically updates when target version is newer.
- Warns and requires explicit confirmation (`y`) when target version is lower than installed.

Two operation modes are supported:

- `--user`: registers a user-level systemd service (no root required)
- `--system`: registers a system-wide service (root required)

### Bootstrap install (fresh machine, no preinstalled binaries)

```bash
./scripts/bootstrap/install.sh --user
```

or:

```bash
sudo ./scripts/bootstrap/install.sh --system
```

Optional flags:

- `--version <tag>` to pin a release (default: `latest`)
- `--repo <owner/name>` to use a fork (default: `basmulder03/git-project-sync`)

The bootstrap script downloads `syncd` and `syncctl`, installs them to:

- user mode: `~/.local/bin`
- system mode: `/usr/local/bin`

then calls `scripts/install.sh` to register and start the service.

### Service registration only

```bash
./scripts/install.sh --user
```

or:

```bash
sudo ./scripts/install.sh --system
```

Environment overrides:

- `BIN_PATH` (default: `~/.local/bin/syncd` for user mode, `/usr/local/bin/syncd` for system mode)
- `CONFIG_PATH` (default: `~/.config/git-project-sync/config.yaml` for user mode, `/etc/git-project-sync/config.yaml` for system mode)

### Uninstall

```bash
./scripts/uninstall.sh --user
```

or:

```bash
sudo ./scripts/uninstall.sh --system
```

### Service files

- User mode: `~/.config/systemd/user/git-project-sync.service`
- System mode: `/etc/systemd/system/git-project-sync.service`

## Windows

### TUI installer (recommended)

Launch `syncsetup.exe` (double-click or from terminal). The same keys are available:

- `m` toggle mode (`user`/`system`)
- `b` bootstrap + install/update
- `r` repair/reinstall
- `i` install/register only
- `u` uninstall/unregister only
- `y` confirm a pending downgrade

Task Scheduler is the default v1 registration mode.

### Bootstrap install (fresh machine, no preinstalled binaries)

```powershell
./scripts/bootstrap/install.ps1 -Mode user
```

or (elevated shell):

```powershell
./scripts/bootstrap/install.ps1 -Mode system
```

Optional parameters:

- `-Version <tag>` to pin a release (default: `latest`)
- `-Repo <owner/name>` to use a fork (default: `basmulder03/git-project-sync`)

The bootstrap script downloads `syncd.exe` and `syncctl.exe`, installs them to:

- user mode: `%LOCALAPPDATA%\git-project-sync\bin`
- system mode: `%ProgramFiles%\git-project-sync`

then calls `scripts/install.ps1` to register and validate the scheduled task.

### Service registration only

```powershell
./scripts/install.ps1 -Mode user
```

or (elevated shell):

```powershell
./scripts/install.ps1 -Mode system
```

Environment overrides:

- `BIN_PATH` (default: `%LOCALAPPDATA%\git-project-sync\bin\syncd.exe` for user mode, `%ProgramFiles%\git-project-sync\syncd.exe` for system mode)
- `CONFIG_PATH` (default: `%APPDATA%\git-project-sync\config.yaml` for user mode, `%ProgramData%\git-project-sync\config.yaml` for system mode)

## Offline/manual fallback

If bootstrap download is not possible, place `syncd` (and optionally `syncctl`) manually on disk, then run `scripts/install.sh` or `scripts/install.ps1` with `BIN_PATH` and `CONFIG_PATH` overrides.

### Uninstall

```powershell
./scripts/uninstall.ps1 -Mode user
```

or (elevated shell):

```powershell
./scripts/uninstall.ps1 -Mode system
```

## Notes

- Install/uninstall flows are designed to be idempotent.
- System mode checks for root privileges and fails fast when insufficient.
- Windows flow uses Task Scheduler and validates registration after install.
