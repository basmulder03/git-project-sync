# PLANS.md

## Milestone 38 — Build Fix (in progress)

- [x] Fix Windows build error in installer PATH registration
- [x] Re-run build to confirm

## Milestone 39 — High-Severity Fixes (in progress)

- [x] Support GitHub user-scope targets (fallback from org endpoint)
- [x] Implement OAuth revocation endpoint calls
- [x] Handle repo rename path moves safely
- [x] Harden repo name sanitization for Windows
- [x] Allow archive moves across devices

## Milestone 40 — Medium-Severity Fixes (in progress)

- [x] Drain retryable HTTP responses before retry
- [x] Add daemon backoff on repeated failures
- [x] Improve lockfile held detection on Windows

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

## Milestone 25 — Robust Daemon Operation (completed)

- [x] Add backoff strategy for repeated provider failures in daemon mode
- [x] Track per-target last-success timestamps in cache for monitoring
- [x] Add tests for backoff timing and last-success update logic

## Milestone 26 — Provider Feature Depth (Webhooks + Limits) (completed)

- [x] Add optional webhook registration per provider (where supported)
- [x] Add rate-limit awareness and retries (provider-specific backoff headers)
- [x] Add tests for rate-limit parsing and retry decisions (mocked)

## Milestone 27 — Sync Integrity Enhancements (completed)

- [x] Add optional verify pass (compare remote HEAD OIDs vs local) in clean repos
- [x] Detect and log ref mismatches beyond default branch (no auto-fix)
- [x] Add tests for verify logic and mismatch logging

## Milestone 28 — Cache/State Management Hardening (completed)

- [x] Introduce cache schema versioning and migration path
- [x] Add cache prune command to remove stale target entries
- [x] Add tests for cache migrations and prune behavior

## Milestone 29 — Token Provisioning Helpers (completed)

- [x] Add CLI helpers to guide users through PAT creation (provider-specific URLs and scopes)
- [x] Add validation checks to confirm required scopes are present (where API allows)
- [x] Add tests for scope guidance and validation logic (mocked)

## Milestone 30 — OAuth / Device Flow Exploration (completed)

- [x] Research provider-supported OAuth/device flows as PAT alternatives
- [x] Prototype one provider flow (feature-flagged)
- [x] Document security implications and fallback to PATs

## Milestone 31 — AzDO OAuth Flow Completion (completed)

- [x] Extend AzDO provider with OAuth device-code flow endpoints and token exchange helpers
- [x] Add CLI command for AzDO OAuth login with clear scope/permission prompts
- [x] Persist AzDO OAuth tokens in keyring; add refresh/expiry handling
- [x] Add tests for device flow happy path and expiry/refresh behavior (mocked)

## Milestone 32 — PAT Management Screens (completed)

- [x] Add TUI screens for PAT listing (per provider/host/scope) with last-verified status
- [x] Add TUI flow for setting/updating PATs with validation and guidance links
- [x] Add TUI flow to validate PATs against required scopes and show results
- [x] Add tests for PAT TUI state transitions and validation outcomes

## Milestone 33 — OAuth GA Hardening (completed)

- [x] Add provider capability gating (feature flags by provider + host)
- [x] Implement robust token refresh + revocation handling across supported providers
- [x] Add audit logging for OAuth events (device start, approval, token refresh, revoke)
- [x] Add tests for refresh/revoke failure handling and audit entries

## Milestone 34 — OAuth GA UX + Docs (completed)

- [x] Update CLI/TUI copy for OAuth GA (warnings, fallbacks, retries, timeouts)
- [x] Add troubleshooting docs for OAuth flows (common errors + recovery steps)
- [x] Add acceptance criteria updates for OAuth GA
- [x] Add end-to-end manual test checklist for OAuth login flows

## Milestone 35 — TUI Main-Flow Guided UX (completed)

- [x] Add guided form hints + inline validation for Config Root, Targets, Tokens
- [x] Normalize main menu labels and footer help text for consistency
- [x] Apply minimal layout polish (headers, spacing, concise summaries)
- [x] Add tests for guidance rendering and validation text

## Milestone 36 — Tray UI + Dashboard (completed)

- [x] Add system tray command with quick actions and dashboard launch
- [x] Add TUI dashboard view with core stats and per-target toggle
- [x] Update docs and acceptance checks for tray/dashboard

## Milestone 37 — Installer Flow + PATH Registration (completed)

- [x] Add CLI install command with optional TUI flow
- [x] Support delayed startup on service install (OS-native)
- [x] Add opt-in PATH registration per OS
- [x] Update docs and acceptance checks

## Notes / Decisions

- Focus: architecture tidy (per user request).
- Breaking CLI/config changes: allowed (major ok).
- Target OS: cross-platform parity.
- Service install helpers: implemented via OS-native registration.
- Token storage: keyring; fallback disabled unless explicitly configured.
- Git implementation: git2; shelling out to git can be added later if needed.
- Docs alignment: spec-first, concise edits only.
- Roadmap focus: Azure DevOps depth first, then provider parity, then sync safety.
