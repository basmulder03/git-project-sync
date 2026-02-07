# PLANS.md — V2 Release Plan

## Objective

Ship **v2** with a cleaner async architecture, reduced coupling, and stable behavior for safe mirroring.

## Scope for V2

- In scope:
  - Provider/core async boundary cleanup
  - Provider HTTP async migration
  - Core sync orchestration async migration
  - CLI async dispatch and handler migration
  - TUI async parity where practical without UI regressions
  - Documentation updates for architecture and breaking changes
- Out of scope:
  - New provider features
  - Major TUI redesign
  - Deep installer UX redesign

## V2 Progress Snapshot

### Completed foundations

- [x] Remove `RemoteRepo.auth` from inventory model
- [x] Resolve provider auth per target via `auth_for_target`
- [x] Deduplicate daemon backoff logic in `mirror-core`
- [x] Convert provider boundary to future-based async interface
- [x] Migrate provider HTTP calls to async `reqwest::Client`
- [x] Replace custom polling executor with Tokio runtime-backed provider `block_on`
- [x] Make core sync orchestration async (`run_sync`, `run_sync_filtered`, inventory load)
- [x] Convert CLI command dispatch and key command handlers to async (`sync`, `daemon`, `token`, `health`, `webhook`)
- [x] Remove CLI command-path `block_on` for provider/sync operations
- [x] Make TUI sync/token helpers async-first and move sync bridge to explicit TUI boundaries
- [x] Remove unused synchronous token validity wrapper
- [x] Keep fmt/clippy/tests green after each migration step

### Remaining bridges

- [ ] Process entry bridge in `crates/mirror-cli/src/main.rs`
- [ ] TUI boundary bridges in:
  - `crates/mirror-cli/src/tui/handle/token.rs`
  - `crates/mirror-cli/src/tui/jobs/start.rs`

## Milestone V2.1 — Final Async Bridge Cleanup

- [ ] Remove process-entry `block_on` bridge
  - Preferred: explicit runtime ownership in `main` without introducing macro-only runtime assumptions
- [ ] Reduce or remove TUI boundary bridges
  - Keep UI responsiveness and background-job behavior unchanged
  - Ensure token validation and sync runs continue to emit same audit events
- [ ] Re-run full quality gates
  - `cargo fmt`
  - `cargo clippy --all-targets --all-features -- -D warnings`
  - `cargo test --all`

## Milestone V2.2 — Core Maintainability Refactor

- [ ] Split `crates/mirror-core/src/cache.rs` into focused modules
  - inventory store
  - runtime/sync status store
  - token/update health store
  - migration helpers
- [ ] Break down `crates/mirror-core/src/sync_engine.rs` into smaller modules
  - orchestration
  - worker execution
  - cache/status reducers
  - error mapping
- [ ] Preserve behavior and compatibility
  - no sync safety rule changes
  - no hidden data-loss paths

## Milestone V2.3 — CLI Surface Cleanup

- [ ] Normalize selector UX in CLI args
  - target-id first
  - provider/scope tuple rules consistent
- [ ] Remove duplicated argument validation paths
- [ ] Ensure help text matches actual precedence and behavior

## Milestone V2.4 — Docs and Release Readiness

- [ ] Update docs to match final v2 architecture and runtime model
  - `README.md`
  - `SPEC.md`
  - `docs/architecture.md`
  - `ACCEPTANCE.md`
- [ ] Add explicit v2 breaking changes section
  - internal model/trait changes
  - migration behavior
- [ ] Final release checklist run
  - fmt/clippy/tests
  - manual smoke checks for sync/daemon/TUI token flows

## Non-Negotiable Behavior (Must Hold)

- Never overwrite dirty working trees
- Fast-forward only on clean repos
- Skip diverged default branches
- Prompt/policy behavior for deleted remotes unchanged
- Staggered scheduling and daemon backoff preserved

## Risks and Mitigations

- Risk: Async refactors regress sync behavior
  - Mitigation: keep safety logic unchanged; test after each step
- Risk: TUI responsiveness regression
  - Mitigation: keep long operations on background channels/jobs
- Risk: Migration drift between docs and code
  - Mitigation: docs updates are mandatory in V2.4 gate

## Decisions and Defaults

- Keep sync safety semantics unchanged for v2
- Breaking internal APIs are allowed in v2
- Prefer incremental refactors with green tests at each step
- Keep bridges only where required by synchronous UI/entry boundaries until fully removed
