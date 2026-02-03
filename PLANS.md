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

- [ ] Stagger buckets (hash(repo_id) % 7)
- [ ] run-once + daemon loop
- [ ] Lock file to prevent concurrent runs

## Milestone 6 — More providers

- [ ] GitHub adapter
- [ ] GitLab adapter

## Notes / Decisions

- Token storage: keyring; fallback disabled unless explicitly configured.
- Git implementation: start with git2; shelling out to git can be added later if needed.
- Missing-remote CLI uses newline-delimited repo ids as the "current" provider set.
