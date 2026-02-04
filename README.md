# Git Project Sync

Mirror repositories from multiple Git providers into a local root folder with safe, fast-forward-only updates.

## Highlights

- Provider-agnostic core with Azure DevOps, GitHub, and GitLab adapters
- Safe sync: never overwrites dirty trees, fast-forward only, logs diverged branches
- Missing remote handling: prompt or policy-based archive/remove/skip
- Staggered auto-sync across 7 days (daemon syncs the current day bucket)
- Tokens stored in OS keychain; config/cache stored in OS AppData equivalents

## Install

Build the CLI with Cargo:

```bash
cargo build --release
```

The binary will be at `target/release/mirror-cli`.

## Quick Start

Initialize config with a mirror root:

```bash
mirror-cli config init --root /path/to/mirrors
```

Add a target:

```bash
# Azure DevOps project scope
mirror-cli target add --provider azure-devops --scope org project

# Azure DevOps org-wide scope
mirror-cli target add --provider azure-devops --scope org

# GitHub org or user
mirror-cli target add --provider github --scope org-or-user

# GitLab group or subgroup path
mirror-cli target add --provider gitlab --scope group subgroup
```

Store a token:

```bash
mirror-cli token set --provider azure-devops --scope org project --token <pat>
mirror-cli token set --provider github --scope org-or-user --token <token>
mirror-cli token set --provider gitlab --scope group --token <token>
```

OAuth device flow (GitHub/Azure DevOps):

```bash
mirror-cli oauth device --provider github --scope org-or-user --client-id <id>
mirror-cli oauth device --provider azure-devops --scope org project --client-id <id> --tenant <tenant>
```

Revoke OAuth tokens:

```bash
mirror-cli oauth revoke --provider github --scope org-or-user
mirror-cli oauth revoke --provider azure-devops --scope org project
```

Run a sync:

```bash
mirror-cli sync
```

## Tray + Dashboard

Run the system tray UI:

```bash
mirror-cli tray
```

The tray menu can open a dashboard TUI or trigger a sync. You can also launch the dashboard directly:

```bash
mirror-cli tui --dashboard
```

## Scope Shapes

- Azure DevOps: `<org>` or `<org>/<project>`
- GitHub: `<org-or-user>`
- GitLab: `<group>/<subgroup>/...`

For Azure DevOps org-wide listing, each repo is mapped to its project name so paths remain `<org>/<project>/<repo>`.

## Mirror Layout

Default layout on disk includes provider prefixes:

```
<root>/
  azure-devops/<org>/<project>/<repo>/
  github/<org>/<repo>/
  gitlab/<group>/<subgroup>/.../<repo>/
  _archive/...
```

## Config, Cache, and Tokens

- Config: `config.json` under the OS config directory (AppData equivalents)
- Cache: `cache.json` under the OS cache directory
- Lock file: `mirror.lock` under the OS runtime directory (or cache dir fallback)
- Tokens: stored in the OS keychain via `keyring`

## Non-interactive Mode

When running unattended, pass a missing-remote policy:

```bash
mirror-cli sync --non-interactive --missing-remote <archive|remove|skip>
```

## Daemon

Run a background loop:

```bash
mirror-cli daemon --missing-remote skip
```

Run one cycle only:

```bash
mirror-cli daemon --run-once --missing-remote skip
```

## Service Install

Register the daemon with the OS:

```bash
mirror-cli service install
mirror-cli service uninstall
```

Notes:

- Linux: installs a systemd user service
- macOS: installs a LaunchAgent
- Windows: installs a Windows service

## Troubleshooting

- If a working tree is dirty, sync will skip it and log the reason
- If the default branch diverges, sync will skip it and log the reason
- If origin is missing or mismatched, sync will update the origin URL before fetch
- For Azure DevOps 401/403/404 responses, the CLI prints a friendly scope/token hint
- OAuth device flow is gated by provider/host allowlist; override with `GIT_PROJECT_SYNC_OAUTH_ALLOW`

### OAuth Troubleshooting

- `OAuth not enabled for <provider> at <host>`: add the host to `GIT_PROJECT_SYNC_OAUTH_ALLOW`
- `device code expired`: restart `oauth device` and complete authorization sooner
- `access denied`: confirm the correct account is used and re-run the flow
- `oauth refresh failed`: revoke the token and re-run `oauth device`
