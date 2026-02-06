# Git Project Sync — Handbook

This handbook is the operator-focused guide to installing, configuring, and running Git Project Sync.

## Purpose

Git Project Sync mirrors repositories from multiple Git providers into a local root folder. It keeps local repos up to date safely, without overwriting dirty working trees and without force-resets. Providers are implemented through adapters so the core engine stays provider-agnostic.

## Quick Install

Build the CLI:

```bash
cargo build --release
```

Run the binary:

```bash
./target/release/mirror-cli --help
```

## Releases

Releases are created manually via the GitHub Actions **Release** workflow. Choose the bump type (major, minor, patch) when dispatching; the workflow updates versions, creates the tag, and publishes binaries.

Quick trigger with the GitHub CLI:

```bash
gh workflow run Release -f bump=patch
```

**Release Checklist**
1. Run `cargo fmt --check`, `cargo clippy --all-targets --all-features -- -D warnings`, and `cargo test --all`.
2. Dispatch the **Release** workflow with the desired bump type.

### PATH behavior

PATH updates are **opt-in** during `mirror-cli install`. If you skip it, you can add it later:

- Windows: add the install folder to the user PATH environment variable.
- macOS/Linux: add a symlink to a PATH location, for example `ln -s <install-dir>/mirror-cli /usr/local/bin/mirror-cli`.

## Configuration

Initialize config with a mirror root:

```bash
mirror-cli config init --root /path/to/mirrors
```

Targets are defined by provider + scope. Scopes differ by provider:

- Azure DevOps: `<org>` or `<org>/<project>`
- GitHub: `<org-or-user>`
- GitLab: `<group>/<subgroup>/...`

Add targets:

```bash
mirror-cli target add --provider azure-devops --scope org project
mirror-cli target add --provider github --scope org-or-user
mirror-cli target add --provider gitlab --scope group subgroup
```

List/remove targets:

```bash
mirror-cli target list
mirror-cli target remove --target-id <id>
```

## Token Management

Tokens are stored in the OS keychain via `keyring`.

### PATs

Store a PAT:

```bash
mirror-cli token set --provider azure-devops --scope org project --token <pat>
```

Tokens are validated on set; invalid or expired PATs are rejected.

Get guidance:

```bash
mirror-cli token guide --provider github --scope org-or-user
```

Validate scopes (when supported):

```bash
mirror-cli token validate --provider github --scope org-or-user
```

## Sync Behavior

For each configured target, the engine:

- Lists remote repos.
- Clones missing repos.
- For existing repos:
  - If working tree clean: fetch + fast-forward the default branch only.
  - If dirty or diverged: skip changes and log.
  - If origin URL missing or mismatched: update origin to expected URL before fetch.
  - If default branch renamed: create missing local branch and log.
  - If default branch missing on remote: skip and log.
  - Detects orphaned local branches (upstream missing) and logs.

Remote repo deleted:

- Interactive: prompt for archive/remove/skip.
- Non-interactive: honors `--missing-remote` policy.

## Cache and Scheduling

- Repo lists are cached with a TTL. Use `--refresh` to bypass.
- Daemon runs only the current day bucket (7-day staggering).
- Backoff is applied per target on repeated failures.
- Target last-success timestamps are tracked for monitoring.

## Running a Sync

```bash
mirror-cli sync
```

Non-interactive policy:

```bash
mirror-cli sync --non-interactive --missing-remote <archive|remove|skip>
```

Verify refs without modifying non-default branches:

```bash
mirror-cli sync --verify
```

## Daemon and Service Install

Daemon loop:

```bash
mirror-cli daemon --missing-remote skip
```

Run one cycle:

```bash
mirror-cli daemon --run-once --missing-remote skip
```

Service install:

```bash
mirror-cli service install
mirror-cli service uninstall
```

OS behavior:

- Linux: installs a systemd user service.
- macOS: installs a LaunchAgent.
- Windows: installs a Windows service.

## Installer Flow

The installer sets up the daemon and optionally registers PATH.

```bash
mirror-cli install
```

Interactive TUI installer:

```bash
mirror-cli install --tui
```

Options:

- `--delayed-start <seconds>`: delayed startup on boot (OS-native).
- `--path <add|skip>`: opt-in PATH registration.

Default install locations (per-user):

- Windows: `%LOCALAPPDATA%\Programs\git-project-sync\mirror-cli.exe`
- macOS: `~/Library/Application Support/git-project-sync/bin/mirror-cli`
- Linux: `~/.local/share/git-project-sync/bin/mirror-cli`

### Default launch behavior

When you run the executable without arguments:

- If **not installed**, it starts the TUI installer.
- If **installed**, it shows the CLI help screen.

Installation is tracked via an install marker file stored under the OS data directory.
The installer also writes an `install.json` manifest to the same location and uses it to detect existing installs.
Only one installer can run at a time (guarded by an install lock file under the same directory).
Re-running the installer replaces the existing install in place and updates the service to point at the new binary.
The installer lock is re-entrant within the same process, so updates can run while the UI holds the lock.

## Updates

Check for updates:

```bash
mirror-cli update --check
```

Apply the latest release:

```bash
mirror-cli update --apply
```

Updates require an existing install. If none is found, run `mirror-cli install` first.

Override the GitHub release repo:

```bash
GIT_PROJECT_SYNC_UPDATE_REPO=owner/repo mirror-cli update --check
```

In the TUI, use the "Update" action from the main menu or press `u` in the installer view.

Auto-update behavior:

- The daemon checks for updates on startup and then daily.
- CLI runs check for updates only if the daemon has not performed a check yet.
- Network failures are logged and do not fail the daemon.
- If updates require elevated permissions, the CLI prompts to re-run with admin/sudo.

PAT validation behavior:

- Tokens are validated on set; invalid or expired PATs are rejected.
- The daemon checks PAT validity daily.
- CLI checks PAT validity only if the daemon has not performed a check yet.

## Dashboard

Dashboard TUI:

```bash
mirror-cli tui --dashboard
```

The dashboard shows core stats and can toggle per-target status with `t`.

## TUI

Launch the terminal UI:

```bash
mirror-cli tui
```

The TUI includes config, targets, tokens, audit logs, and dashboard views.

## Logs and Audit

Audit logs are JSONL files stored under the OS data directory (`audit/`). Entries include command, status, and context. Rotate when size limit is reached.

## Troubleshooting

Common issues:

- Dirty working tree: repo is skipped by design.
- Diverged default branch: repo is skipped by design.
- Missing origin: origin URL is restored automatically.
- PATH not updated: rerun `mirror-cli install --path add` or update PATH manually.

## Safety Guarantees

- No force resets.
- No overwrite of dirty working trees.
- Fast-forward only for default branch.
- Diverged or missing default branch is logged and skipped.
