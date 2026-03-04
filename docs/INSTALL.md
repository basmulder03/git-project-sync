# Install and Service Registration

This guide covers Linux install/uninstall in v1.

## Linux

Two operation modes are supported:

- `--user`: registers a user-level systemd service (no root required)
- `--system`: registers a system-wide service (root required)

### Install

```bash
./scripts/install.sh --user
```

or:

```bash
sudo ./scripts/install.sh --system
```

Environment overrides:

- `BIN_PATH` (default: `/usr/local/bin/syncd`)
- `CONFIG_PATH` (default: `$HOME/.config/git-project-sync/config.yaml`)

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

Task Scheduler is the default v1 registration mode.

### Install

```powershell
./scripts/install.ps1 -Mode user
```

or (elevated shell):

```powershell
./scripts/install.ps1 -Mode system
```

Environment overrides:

- `BIN_PATH` (default: `%ProgramFiles%\git-project-sync\syncd.exe`)
- `CONFIG_PATH` (default: `%APPDATA%\git-project-sync\config.yaml`)

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
