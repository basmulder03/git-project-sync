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

### PATH behavior

This project does **not** modify PATH on any OS. The service installer only registers background services. If you want `mirror-cli` on PATH, use OS-specific steps:

- Windows: add the release folder (for example `C:\path\to\repo\target\release`) to the user PATH environment variable.
- macOS/Linux: add a symlink to a PATH location, for example `ln -s /path/to/repo/target/release/mirror-cli /usr/local/bin/mirror-cli`.

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

Get guidance:

```bash
mirror-cli token guide --provider github --scope org-or-user
```

Validate scopes (when supported):

```bash
mirror-cli token validate --provider github --scope org-or-user
```

### OAuth Device Flow

Device flow is supported for GitHub and Azure DevOps.

```bash
mirror-cli oauth device --provider github --scope org-or-user --client-id <id>
mirror-cli oauth device --provider azure-devops --scope org project --client-id <id> --tenant <tenant>
```

OAuth is gated by provider and host. You can override the allowlist:

```
GIT_PROJECT_SYNC_OAUTH_ALLOW=github=github.com;azure-devops=dev.azure.com,visualstudio.com
```

Revoke OAuth tokens:

```bash
mirror-cli oauth revoke --provider github --scope org-or-user
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

## Tray + Dashboard

System tray UI:

```bash
mirror-cli tray
```

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
- OAuth not enabled: add host to `GIT_PROJECT_SYNC_OAUTH_ALLOW`.
- Device code expired: re-run `oauth device` and complete authorization sooner.
- OAuth refresh failed: revoke token and re-run device flow.
- PATH not updated: rerun `mirror-cli install --path add` or update PATH manually.

## Safety Guarantees

- No force resets.
- No overwrite of dirty working trees.
- Fast-forward only for default branch.
- Diverged or missing default branch is logged and skipped.
