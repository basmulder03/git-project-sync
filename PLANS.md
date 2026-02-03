# PLANS.md

## Milestone 0 — Repo scaffolding

- [x] Create Rust workspace + crates
- [x] Add basic CI commands (fmt, clippy, test)

## Milestone 1 — Core contracts (provider-agnostic)

- [x] Define normalized models: RemoteRepo, ProviderScope, ProviderTarget
- [x] Define RepoProvider trait (list_repos, validate_auth, optional get_repo)
- [x] Define filesystem mapping (root/provider/scope/repo)

## Milestone 2 — Azure DevOps adapter (first provider)

- [x] Implement AzdoProvider list_repos for org+project
- [x] PAT auth via keyring
- [x] Minimal config for target: { org, project }

## Milestone 3 — Git engine (safe sync)

- [x] Clone missing repos
- [x] Detect clean working tree
- [x] Fetch origin
- [x] Fast-forward default branch only

## Milestone 4 — Cache + deleted remote handling

- [x] Cache seen repo IDs + metadata
- [x] Detect deleted remote repos
- [x] Prompt archive/remove/skip (CLI + interactive)
- [x] Implement archive move

## Milestone 5 — Scheduler + daemon

- [x] Stagger buckets (hash(repo_id) % 7)
- [x] run-once + daemon loop
- [x] Lock file to prevent concurrent runs

## Milestone 6 — More providers

- [x] GitHub adapter
- [x] GitLab adapter

## Milestone 7 — Core sync pipeline

- [x] Wire provider listing + cache update + git sync into a core engine loop
- [x] Integrate missing-remote policy handling into the engine
- [x] Record last sync timestamps in cache

## Milestone 8 — CLI wiring

- [x] Add config model + load/save (AppData path)
- [x] Add commands: init/configure root, add target, set token
- [x] Add sync commands: sync all / sync target / sync repo
- [x] Add non-interactive policy flags for missing remotes on sync

## Milestone 9 — Background integration

- [x] Connect daemon runner to core sync pipeline
- [x] Add run-once CLI using real sync job (not placeholder)
- [x] Design for service install/uninstall (systemd/launchd/Windows)

## Milestone 10 — Hardening

- [x] Add structured logging fields (provider, scope, repo_id, path) across sync flows
- [x] Add more unit tests for edge cases (diverged branch, missing remote ref)
- [x] Improve errors and user-facing messages

## Milestone 11 — Service Install Helpers

- [ ] Implement systemd user service install/uninstall
- [ ] Implement launchd agent install/uninstall
- [ ] Implement Windows service install/uninstall

## Notes / Decisions

- Token storage: keyring; fallback disabled unless explicitly configured.
- Git implementation: start with git2; shelling out to git can be added later if needed.
- Missing-remote CLI uses newline-delimited repo ids as the "current" provider set.
- When only 1-2 milestones remain unchecked, add new milestones to keep the plan rolling forward.
- For future milestones: create a descriptive commit and push to remote after each milestone is completed.
