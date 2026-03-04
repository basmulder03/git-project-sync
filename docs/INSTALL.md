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

## Notes

- Install/uninstall flows are designed to be idempotent.
- System mode checks for root privileges and fails fast when insufficient.
- Windows installer flow is tracked separately in Sprint 2 task S2-T05.
