# Git Project Sync

Mirror repositories from multiple Git providers into a local root folder with safe, fast-forward-only updates.

## Highlights

- Provider-agnostic core with Azure DevOps, GitHub, and GitLab adapters
- Safe sync: never overwrites dirty trees, fast-forward only, logs diverged branches
- Missing remote handling: prompt or policy-based archive/remove/skip
- Staggered auto-sync across 7 days (daemon syncs the current day bucket)
- Tokens stored in OS keychain; config/cache stored in OS AppData equivalents

## Documentation

- `docs/handbook.md` — Operator guide
- `docs/architecture.md` — System architecture
- `docs/decisions.md` — Architectural decisions

## Install

Build the CLI with Cargo:

```bash
cargo build --release
```

The binary will be at `target/release/mirror-cli`.

## Releases

Push a release-ready version to `crates/mirror-cli/Cargo.toml` on `main` to trigger the GitHub Release workflow and prebuilt binaries.

**Release Checklist**
1. Update versions in `crates/mirror-cli/Cargo.toml`, `crates/mirror-core/Cargo.toml`, and `crates/mirror-providers/Cargo.toml`.
2. Run `cargo fmt --check`, `cargo clippy --all-targets --all-features -- -D warnings`, and `cargo test --all`.
3. Merge to `main` and let the Release workflow create the tag and binaries.

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

Live status for sync:

```bash
mirror-cli sync --status
```

## Updates

Check for updates:

```bash
mirror-cli update --check
```

Apply the latest release:

```bash
mirror-cli update --apply
```

Override the GitHub release repo:

```bash
GIT_PROJECT_SYNC_UPDATE_REPO=owner/repo mirror-cli update --check
```

## Dashboard

Launch the dashboard TUI:

```bash
mirror-cli tui --dashboard
```

In the TUI:
- Dashboard view: press `s` for Sync Status
- Installer view: press `s` for Installer Status

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

## Install Flow

Run the guided installer:

```bash
mirror-cli install
```

Use the TUI installer:

```bash
mirror-cli install --tui
```

Optional flags:

- `--delayed-start <seconds>`: delay startup on boot (OS-native where supported)
- `--path <add|skip>`: opt-in PATH registration
- `--status`: show install status (path, task/service state, PATH)

Windows task helpers (when installed):

```bash
schtasks /Query /TN git-project-sync
schtasks /Run /TN git-project-sync
```

Default launch behavior:

- Running `mirror-cli` with no arguments will open the installer if not installed.
- Once installed, running `mirror-cli` without arguments shows the CLI help.

Notes:

- The installer copies `mirror-cli` into the OS default per-user install location and reuses it for the service.
- Re-running `mirror-cli install` replaces the existing install in place.
- Linux: installs a systemd user service
- macOS: installs a LaunchAgent
- Windows: installs a Scheduled Task (Task Scheduler)
- Only one installer can run at a time (guarded by a lock file under the OS app data directory).

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
