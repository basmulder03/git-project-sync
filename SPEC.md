# SPEC.md — Multi-provider repo mirror tool (Rust)

## Core behavior

- Root folder configured. Each provider and scope gets its own directory; each repo under it.
- For every configured target (provider + scope), list remote repos.
- If local repo missing: clone.
- If local exists:
  - If working tree clean: fetch and fast-forward default branch only.
  - If dirty or diverged: do not modify working tree; log.
- If remote repo deleted:
  - Ask user: remove / archive / skip
  - Archive moves repo to archive root preserving provider/scope/repo layout.
- If only a branch remote disappears: do nothing special.

## Caching & scheduling

- Cache repo inventory and last sync timestamps in AppData (non-sensitive).
- Token stored in OS keychain.
- Auto-sync staggered over 7 days using stable hash bucketing.
- Background daemon runs periodically; also provide run-once.

## Providers

- Adapter pattern:
  - Core engine only depends on provider trait.
  - Providers implement listing repos + returning normalized RemoteRepo info.
- Must support: Azure DevOps, GitHub, GitLab (others later).

## CLI

- init/configure root + targets
- set auth token per provider/host
- sync all / sync target / sync repo
- non-interactive mode and missing-remote policy flags
