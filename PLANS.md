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

## Milestone 16 — Documentation & README (completed)

- [x] Create README with overview, install, and usage examples
- [x] Document config file structure and cache/token storage locations
- [x] Add provider scope examples and mirror folder layout
- [x] Add daemon/service usage and troubleshooting notes

## Milestone 17 — Terminal GUI: Config & Targets (completed)

- [x] Add a TUI for config initialization and root selection
- [x] Add a TUI for managing targets (add/list/remove) with scope validation
- [x] Add a TUI for token setup per provider/host/scope
- [x] Add tests for TUI state transitions and validation logic

## Milestone 18 — Audit Logging & Operational Telemetry (completed)

- [x] Add structured audit log entries for startup, commands, sync runs, and errors
- [x] Persist audit logs to OS app data directory with rotation policy
- [x] Include audit IDs in CLI/TUI output for traceability
- [x] Add tests for audit log schema and write/rotation behavior

## Milestone 19 — Terminal GUI: Service Installer (completed)

- [x] Add a TUI for install/uninstall with OS-specific status summaries
- [x] Provide clear confirmation prompts and error views for failures
- [x] Add tests for installer TUI state and error handling

## Milestone 20 — Terminal GUI: Audit Log Viewer (completed)

- [x] Add a TUI for browsing audit logs with filters (time, command, status)
- [x] Include a failure-focused view with error details and remediation hints
- [x] Add tests for log parsing and filter behavior

## Milestone 21 — Provider Auth + Scope Diagnostics (completed)

- [x] Add provider-level health checks to validate tokens/scopes (AzDO/GitHub/GitLab)
- [x] Improve CLI error messages for missing scopes or invalid hosts with provider hints
- [x] Add tests for auth failures and scope validation across providers

## Milestone 22 — Sync Safety Edge Cases (completed)

- [x] Handle missing default branch on remote (log and skip, no local changes)
- [x] Detect and log orphaned local branches after remote deletion
- [x] Add tests for missing default branch and orphaned branch detection

## Milestone 23 — Repo Inventory Caching + Incremental Sync (completed)

- [x] Cache remote repo lists per target with TTL to reduce API calls
- [x] Add `--refresh` to bypass repo list cache per target/provider
- [x] Add tests for cache read/write and TTL invalidation

## Milestone 24 — Provider Feature Parity (Metadata) (completed)

- [x] Capture repo metadata (archived/disabled status where available)
- [x] Skip archived/disabled repos by default; add `--include-archived`
- [x] Add tests for metadata parsing and skip behavior

## Milestone 25 — Robust Daemon Operation

- [ ] Add backoff strategy for repeated provider failures in daemon mode
- [ ] Track per-target last-success timestamps in cache for monitoring
- [ ] Add tests for backoff timing and last-success update logic

## Notes / Decisions

- Focus: architecture tidy (per user request).
- Breaking CLI/config changes: allowed (major ok).
- Target OS: cross-platform parity.
- Service install helpers: implemented via OS-native registration.
- Token storage: keyring; fallback disabled unless explicitly configured.
- Git implementation: git2; shelling out to git can be added later if needed.
- Docs alignment: spec-first, concise edits only.
- Roadmap focus: Azure DevOps depth first, then provider parity, then sync safety.
