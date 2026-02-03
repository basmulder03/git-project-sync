# PLANS.md

## Milestone 12 — Architecture cleanup (completed)

- [x] Add provider spec registry to centralize host/scope/account logic
- [x] Refactor sync engine into plan/execute flow with summary reporting
- [x] Introduce config v2 + migration path
- [x] CLI rework to new commands + config v2
- [x] Path sanitization for repo names
- [x] Update tests + acceptance notes

## Milestone 11 — Service Install Helpers (completed)

- [x] Implement systemd user service install/uninstall
- [x] Implement launchd agent install/uninstall
- [x] Implement Windows service install/uninstall

## Milestone 13 — Azure DevOps Depth (completed)

- [x] Implement AzDO pagination and continuation tokens for repo listing
- [x] Support project-scoped vs org-scoped listing (confirm and document scope behavior)
- [x] Add AzDO-specific error handling for auth failures (401/403) and not-found scopes
- [x] Add tests for AzDO pagination + scope parsing + auth failure mapping

## Milestone 14 — Provider Parity (GitHub/GitLab) (completed)

- [x] Implement pagination for GitHub and GitLab repo listings
- [x] Normalize default branch handling across providers (explicit fallbacks if API omits)
- [x] Add tests for GitHub/GitLab listing + pagination + scope validation

## Milestone 15 — Sync Safety & Robustness (completed)

- [x] Handle missing origin remote explicitly (create or repair remote to expected URL)
- [x] Handle default-branch rename detection (if remote default changes)
- [x] Improve logging + summary for skipped repos (dirty, diverged, missing-remote)
- [x] Add tests for missing origin, default-branch change, and logging outcomes

## Milestone 16 — Documentation & README

- [ ] Create README with overview, install, and usage examples
- [ ] Document config file structure and cache/token storage locations
- [ ] Add provider scope examples and mirror folder layout
- [ ] Add daemon/service usage and troubleshooting notes

## Notes / Decisions

- Focus: architecture tidy (per user request).
- Breaking CLI/config changes: allowed (major ok).
- Target OS: cross-platform parity.
- Service install helpers: implemented via OS-native registration.
- Token storage: keyring; fallback disabled unless explicitly configured.
- Git implementation: git2; shelling out to git can be added later if needed.
- Docs alignment: spec-first, concise edits only.
- Roadmap focus: Azure DevOps depth first, then provider parity, then sync safety.
