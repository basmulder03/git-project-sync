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

- [x] Process entry bridge replaced with explicit Tokio runtime ownership in `crates/mirror-cli/src/main.rs`
- [x] TUI boundary bridges moved from provider helper shim to explicit TUI runtime helper:
  - `crates/mirror-cli/src/tui/handle/token.rs`
  - `crates/mirror-cli/src/tui/jobs/start.rs`

## Milestone V2.1 — Final Async Bridge Cleanup

- [x] Remove process-entry provider bridge
  - Implemented explicit runtime ownership in `main` using Tokio builder
- [x] Reduce/remove TUI provider bridges
  - Replaced provider shim calls with TUI runtime helper at explicit sync boundaries
  - Preserved UI responsiveness and background-job behavior
  - Preserved token/sync audit behavior
- [x] Re-run full quality gates
  - `cargo fmt`
  - `cargo clippy --all-targets --all-features -- -D warnings`
  - `cargo test --all`

## Milestone V2.2 — Core Maintainability Refactor

- [x] Split `crates/mirror-core/src/cache.rs` internals into focused modules
  - `cache/migration.rs` for schema migration structs/functions
  - `cache/backoff.rs` for retry-delay policy logic
  - preserve external `cache` API shape
- [x] Split cache free-function concerns into dedicated modules and re-export via facade
  - `cache/inventory.rs` for prune/inventory maintenance
  - `cache/runtime.rs` for target backoff/success runtime functions
  - `cache/health.rs` for token/update check and status functions
- [ ] Continue cache split into dedicated store-facing modules
  - inventory store
  - runtime/sync status store
  - token/update health store
- [ ] Break down `crates/mirror-core/src/sync_engine.rs` into smaller modules
  - orchestration
  - worker execution
  - cache/status reducers
  - error mapping
- [x] Extract duplicated sync outcome application from serial/parallel paths into shared helper module
  - added `crates/mirror-core/src/sync_engine_apply.rs`
  - centralized success/failure cache+status+audit updates
  - preserved existing sync semantics and logging fields
- [x] Extract sync worker execution (serial + threaded) into dedicated module
  - added `crates/mirror-core/src/sync_engine_workers.rs`
  - `run_sync_filtered` now delegates repo execution to worker helper
  - kept orchestration decisions in `sync_engine.rs`
- [x] Extract missing-repo detection + status emission flow into dedicated helper
  - added `crates/mirror-core/src/sync_engine_missing_events.rs`
  - `run_sync_filtered` now delegates missing-remote event mapping/emission
  - preserved missing-policy behavior and emitted action semantics
- [x] Add focused tests for outcome application helpers
  - success path records repo + `last_sync` and emits `up_to_date` action
  - failure path increments failed counter and emits `failed` action
- [x] Extract repo work-item path/rename preparation into dedicated helper
  - added `crates/mirror-core/src/sync_engine_work_items.rs`
  - moved rename-path/missing-path logging flow out of `run_sync_filtered`
  - kept path mapping and rename behavior unchanged
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
- Keep runtime ownership explicit at synchronous UI/entry boundaries where async cannot be threaded through safely
