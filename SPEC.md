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
- If remote repo deleted:
  - Ask user: remove / archive / skip
  - Archive moves repo to archive root preserving provider/scope/repo layout.
- If only a branch remote disappears: do nothing special.

## Caching & scheduling

- Cache repo inventory and last sync timestamps in AppData equivalents (non-sensitive).
- Token stored in OS keychain.
- Auto-sync staggered over 7 days using stable hash bucketing.
- Background daemon runs periodically; also provide run-once. Daemon syncs only the current day bucket.

## Providers

- Adapter pattern:
  - Core engine only depends on provider trait.
  - Providers implement listing repos + returning normalized RemoteRepo info.
- Must support: Azure DevOps, GitHub, GitLab (others later).

## CLI

- config init root + targets
- token set per provider/host
- target add/list/remove
- sync all / sync target / sync repo
- non-interactive mode and missing-remote policy flags (archive/remove/skip)
- service install/uninstall (systemd user service, launchd agent, Windows service)
