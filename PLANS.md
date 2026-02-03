# PLANS.md

## Milestone 0 — Repo scaffolding

- [ ] Create Rust workspace + crates
- [ ] Add basic CI commands (fmt, clippy, test)

## Milestone 1 — Core contracts (provider-agnostic)

- [ ] Define normalized models: RemoteRepo, ProviderScope, ProviderTarget
- [ ] Define RepoProvider trait (list_repos, validate_auth, optional get_repo)
- [ ] Define filesystem mapping (root/provider/scope/repo)

## Milestone 2 — Azure DevOps adapter (first provider)

- [ ] Implement AzdoProvider list_repos for org+project
- [ ] PAT auth via keyring
- [ ] Minimal config for target: { org, project }

## Milestone 3 — Git engine (safe sync)

- [ ] Clone missing repos
- [ ] Detect clean working tree
- [ ] Fetch origin
- [ ] Fast-forward default branch only

## Milestone 4 — Cache + deleted remote handling

- [ ] Cache seen repo IDs + metadata
- [ ] Detect deleted remote repos
- [ ] Prompt archive/remove/skip (CLI + interactive)
- [ ] Implement archive move

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
