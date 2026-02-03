# PLANS.md

## Milestone 12 — Architecture cleanup (current session)

- [x] Add provider spec registry to centralize host/scope/account logic
- [x] Refactor sync engine into plan/execute flow with summary reporting
- [x] Introduce config v2 + migration path
- [x] CLI rework to new commands + config v2
- [x] Path sanitization for repo names
- [x] Update tests + acceptance notes

## Milestone 11 — Service Install Helpers

- [ ] Implement systemd user service install/uninstall
- [ ] Implement launchd agent install/uninstall
- [ ] Implement Windows service install/uninstall

## Notes / Decisions

- Focus: architecture tidy (per user request).
- Breaking CLI/config changes: allowed (major ok).
- Target OS: cross-platform parity.
- Service install helpers: deferred to later milestone.
- Token storage: keyring; fallback disabled unless explicitly configured.
- Git implementation: git2; shelling out to git can be added later if needed.
