# AGENTS.md — Repo instructions for coding agents (Codex)

## Goal

Build a Rust tool that mirrors repositories from multiple Git providers (Azure DevOps, GitHub, GitLab, …) into a configured root folder, keeping local repos up to date safely, with caching + staggered auto-sync. Providers must be implemented via an adapter pattern so the core engine stays provider-agnostic. A “target” is `provider + scope`.

## Non-negotiables

- Language: Rust (edition 2024 if available, otherwise 2021).
- Multi-provider: implement a provider trait + adapters. Azure DevOps first.
- Safe git sync:
  - Never overwrite dirty working trees.
  - If clean: fetch + fast-forward only (no force reset).
  - If default branch diverged: leave untouched and log.
- Deleted remote repo handling:
  - If repo exists locally but is gone remotely: prompt (archive/remove/skip).
  - If only a remote branch is gone: do nothing special.
- Caching:
  - Store non-sensitive config/cache/logs under OS AppData equivalents.
  - Store tokens securely (OS keychain via `keyring`).
- Scheduler:
  - Auto-update staggered across 7 days (hash-bucket by repo ID).
  - Daemon syncs only the current day bucket.
- CLI:
  - Manual refresh for provider/target/project/repo.
  - `--non-interactive` and policy flags for missing remotes.
- Background app:
  - Provide a daemon mode.
  - Provide install/uninstall to register background execution (systemd user service, launchd agent, Windows service) — can be phased, but design for it.

## Repository layout

Use a workspace:

- crates/mirror-core      (provider-agnostic engine)
- crates/mirror-providers (provider trait + adapters)
- crates/mirror-cli       (CLI + wiring + logging)
- crates/mirror-service   (optional, OS service helpers; can land later)

## Folder layout on disk

Default local mirror layout MUST include provider prefix to avoid collisions:
<root>/
  azure-devops/<org>/<project>/<repo>/
  github/<org>/<repo>/
  gitlab/<group>/<subgroup>/.../<repo>/
  _archive/...

## Engineering conventions

- Prefer small, testable modules.
- Use `tracing` for logs; include clear structured fields (provider, scope, repo_id, path).
- Errors with `anyhow` at boundaries, `thiserror` for internal typed errors where useful.
- Add unit tests for: path mapping, stagger bucketing, “clean tree” logic (mock), cache read/write.
- All network calls must be behind provider adapters.

## Planning contract

When implementing multi-step work, keep an up-to-date `PLANS.md`:

- Create/Update PLANS.md at start of a session
- Break work into small milestones with checkboxes
- Record decisions/assumptions and any follow-ups

If anything is ambiguous, make a reasonable default and document it in PLANS.md rather than blocking.
