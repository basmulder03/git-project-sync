# Git Project Sync

Mirror repositories from multiple Git providers into a local root folder with safe, fast-forward-only updates.

## Highlights

- Provider-agnostic core with Azure DevOps, GitHub, and GitLab adapters
- Safe sync: never overwrites dirty trees, fast-forward only, logs diverged branches
- Missing remote handling: prompt or policy-based archive/remove/skip
- Staggered auto-sync across 7 days (daemon syncs the current day bucket)
- Tokens stored in OS keychain; config/cache stored in OS AppData equivalents
- Localization support with configurable language (`en-001`, `en-US`, `en-GB`, `nl`, `af`)

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

Releases are created manually via the GitHub Actions **Release** workflow. Choose the semver bump (major, minor, patch) when dispatching. The workflow computes the next version from the latest `vX.Y.Z` tag, updates `crates/mirror-cli/Cargo.toml`, `crates/mirror-core/Cargo.toml`, and `crates/mirror-providers/Cargo.toml`, commits the bump, creates the tag, and publishes release binaries for Windows, macOS, and Linux.

Quick trigger with the GitHub CLI:

```bash
gh workflow run Release -f bump=patch
```

**Release Checklist**
1. Run `cargo fmt --check`, `cargo clippy --all-targets --all-features -- -D warnings`, and `cargo test --all`.
2. Dispatch the **Release** workflow with the desired bump type.

## v2 Breaking Changes

### Internal API/architecture changes

- Provider inventory records are credential-free:
  - `RemoteRepo.auth` was removed from the inventory model.
  - Credentials are resolved per target during execution via provider `auth_for_target`.
- Provider trait boundary is async-first:
  - Core/provider interaction is future-based at the adapter boundary.
  - Provider adapters use async `reqwest::Client` for HTTP and retry handling.
- Sync engine internals were split into focused modules:
  - orchestration, worker execution, missing-remote flow, outcome reducers, work-item preparation.
  - this is an intentional internal refactor; behavior is preserved.
- Runtime ownership is explicit:
  - CLI process entry owns the Tokio runtime.
  - synchronous TUI boundaries use a dedicated TUI runtime helper.

### CLI/TUI behavior changes

- Selector precedence is now explicit and consistent for `sync` and `health`:
  - `--target-id` takes precedence over `--provider/--scope`.
  - using both now emits a warning and applies `--target-id`.
- TUI navigation/overflow behavior was unified:
  - `Esc` returns to previous screen consistently.
  - overflow content uses `PgUp/PgDn/Home/End` scrolling across screens.

## Quick Start

Initialize config with a mirror root:

```bash
mirror-cli config init --root /path/to/mirrors
```

Set display language:

```bash
mirror-cli config language set --lang en-001
mirror-cli --lang nl sync
```

Language precedence: `--lang` > `MIRROR_LANG` > `config.language` > `en-001`.

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

Tokens are validated on set; invalid or expired PATs are rejected.

Diagnose keyring/session issues:

```bash
mirror-cli token doctor --provider github --scope org-or-user
```


Run a sync:

```bash
mirror-cli sync
```

Force-refresh all configured targets and repos (ignores target/scope/repo filters):

```bash
mirror-cli sync --force-refresh-all
```

Live status for sync:

```bash
mirror-cli sync --status
```

## Updates

Check for updates:

```bash
mirror-cli update --check-only
```

Check updates alongside any command:

```bash
mirror-cli sync --check-updates
```

Apply the latest release:

```bash
mirror-cli update --apply
```

Override the GitHub release repo:

```bash
GIT_PROJECT_SYNC_UPDATE_REPO=owner/repo mirror-cli update --check-only
```

Auto-update behavior:

- The daemon checks for updates on startup and then daily.
- CLI runs check for updates only if the daemon has not performed a check yet.
- Network failures are logged and do not fail the daemon.
- If updates require elevated permissions, the CLI prompts to re-run and triggers the OS admin-privileges prompt (UAC/sudo/etc.).
- After applying updates, the running CLI/TUI/daemon restarts to load the new version.

PAT validation behavior:

- Tokens are validated on set; invalid or expired PATs are rejected.
- The daemon checks PAT validity daily.
- CLI checks PAT validity only if the daemon has not performed a check yet.

## Dashboard

Launch the dashboard TUI:

```bash
mirror-cli tui --dashboard
```

In the TUI:
- Dashboard view: press `s` for Sync Status
- Dashboard view: press `f` for Force Refresh All
- Main menu: open `Language` to switch locale and persist it
- Setup view: press `s` for Setup Status
- General navigation: `Esc` goes back to previous screen
- Overflow scrolling: `PgUp/PgDn/Home/End`

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

## Release Readiness (v2)

Before cutting a release:

1. Run quality gates:
   - `cargo fmt --check`
   - `cargo clippy --all-targets --all-features -- -D warnings`
   - `cargo test --all`
2. Run manual smoke checks:
   - sync (normal + `--force-refresh-all`)
   - daemon (`--run-once`)
   - token set/validate/doctor flows
   - TUI navigation and status screens

- If a working tree is dirty, sync will skip it and log the reason
- If the default branch diverges, sync will skip it and log the reason
- If origin is missing or mismatched, sync will update the origin URL before fetch
- For Azure DevOps 401/403/404 responses, the CLI prints a friendly scope/token hint
