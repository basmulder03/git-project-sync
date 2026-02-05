# Git Project Sync — Architecture

This document describes the architecture, module boundaries, and key data flows.

## Workspace layout

- `crates/mirror-core`: provider-agnostic sync engine, config, cache, scheduler, audit
- `crates/mirror-providers`: provider adapters (Azure DevOps, GitHub, GitLab)
- `crates/mirror-cli`: CLI and TUI wiring, command handlers

## Provider adapter pattern

The core engine depends on the `RepoProvider` trait. Providers implement:

- `list_repos` to enumerate remote repositories
- `validate_auth` and `health_check` to verify credentials
- `auth_for_target` to return git auth info
- `token_scopes` to validate scopes (when supported)
- `register_webhook` to configure webhooks (if supported)

This isolates provider-specific APIs from the core sync logic.

## Data flow: sync

1. Load config and cache.
2. For each target:
   - Check daemon backoff and day bucket.
   - List remote repos (with cache TTL).
   - Plan sync operations.
3. For each repo:
   - If missing locally: clone.
   - If present and clean: fetch + fast-forward default branch only.
   - If dirty or diverged: skip and log.
4. Record audit events and update cache (last sync, backoff, last success).

## Git safety rules

- Dirty working trees are never modified.
- Diverged default branches are never modified.
- Only fast-forward is applied to default branch.
- Non-default branches are not modified during verify.

## Cache + scheduling

- Cache persists repo inventory per target with TTL.
- `target_last_success` and `target_backoff_until` are stored in cache.
- Scheduler uses a stable hash bucket (0–6) to stagger repos across days.

## Audit logging

Audit entries are JSONL and include:

- timestamp
- command
- status
- provider/scope context
- errors (if any)

Logs are rotated by file size and stored under OS data directory.

## CLI and TUI

- CLI is built with clap subcommands and maps directly to core functions.
- TUI is built with ratatui/crossterm and provides forms for config, targets, tokens, audit, and dashboard.
- TUI includes guided validation hints and inline feedback.

## Dashboard

The dashboard summarizes:

- target counts
- backoff status
- last success
- audit entry counts

## Token storage

Tokens are stored in the OS keychain via `keyring` as provider PATs.

## Service installers

Service install helpers are OS-specific:

- Linux: systemd user units
- macOS: LaunchAgents
- Windows: Windows service helpers

## Known limitations

- PATH modification is opt-in during install.
