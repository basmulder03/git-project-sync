# SPEC.md — Multi-provider repo mirror tool (Rust)

## Core behavior

- Target = provider + scope (scope varies by provider).
- Scope shapes: Azure DevOps <org> or <org>/<project>; GitHub <org-or-user>; GitLab <group>/<subgroup>/... (one or more segments).

- Root folder configured. Each provider and scope gets its own directory; each repo under it.
- For every configured target, list remote repos.
- If local repo missing: clone.
- If local exists:
  - If working tree clean: fetch and fast-forward default branch only.
  - If dirty or default branch diverged: do not modify working tree; log.
  - If origin remote missing or mismatched: create/update origin to expected URL before fetch.
  - If default branch name changes: create missing local branch and log.
  - If default branch is missing on remote: skip and log (no local changes).
  - Detect orphaned local branches (upstream missing) and log.
- If remote repo deleted:
  - Ask user: remove / archive / skip
  - Archive moves repo to archive root preserving provider/scope/repo layout.
- If only a branch remote disappears: do nothing special.
- Runtime model:
  - CLI process entry owns Tokio runtime.
  - TUI sync boundaries use a dedicated runtime helper.

## Caching & scheduling

- Cache repo inventory and last sync timestamps in AppData equivalents (non-sensitive).
- Token stored in OS keychain.
- Audit logs stored as JSONL under OS data directory with size-based rotation.
- Repo list cache has a short TTL; `--refresh` bypasses it.
- Daemon applies per-target backoff on repeated failures and tracks last-success timestamps.
- Auto-sync staggered over 7 days using stable hash bucketing.
- Background daemon runs periodically; also provide run-once. Daemon syncs only the current day bucket.

## Providers

- Adapter pattern:
  - Core engine only depends on provider trait.
  - Providers implement listing repos + returning normalized RemoteRepo info.
  - Provider credentials are resolved per target at sync execution time (not stored on repo inventory records).
- Must support: Azure DevOps, GitHub, GitLab (others later).

## CLI

- config init root + targets
- token set per provider/host
- target add/list/remove
- sync all / sync target / sync repo
- selector precedence:
  - `--target-id` takes precedence over `--provider/--scope`
  - if `--target-id` is set with provider/scope selectors, provider/scope are ignored with warning
- non-interactive mode and missing-remote policy flags (archive/remove/skip)
- service install/uninstall (systemd user service, launchd agent, Windows service)
- health check per target (validate auth + scope)
- `--include-archived` to sync archived/disabled repos (skipped by default)
- `webhook register` to configure provider webhooks where supported
- `sync --verify` to compare local refs with upstreams without modifying non-default branches
- `cache prune` to remove cached entries for deleted targets
- `token guide` and `token validate` to help create and validate PATs
- localization:
  - all user-visible CLI/TUI text is translatable
  - supported locales: `en-001`, `en-US`, `en-GB`, `nl`, `af`
  - language precedence: `--lang` > `MIRROR_LANG` > config `language` > `en-001`
  - `config language set/show` manages persisted language preference
- `tui --dashboard` to open the status dashboard directly
- `install` to run guided installation (service + optional PATH)
- Installer copies the binary to the OS default per-user install location and re-runs service registration from that path.
- Auto-update: daemon checks for updates on startup and daily; CLI checks only if daemon has not run yet. Network failures are logged and non-fatal. Updates prompt for elevation when required.
- PAT validation: tokens are validated on set; daemon performs daily validity checks and logs/prints when tokens are invalid or expired. CLI checks only if the daemon has not yet run.

## TUI consistency contract

- `Esc` returns to previous screen consistently.
- Overflow content is scrollable with `PgUp/PgDn/Home/End`.
- Footer help text reflects screen-local actions plus global scroll/back behavior.
