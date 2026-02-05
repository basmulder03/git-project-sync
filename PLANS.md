# PLANS.md

## Milestone 51 — GitHub CI + Release + Self-Update (in progress)

- [x] Add GitHub Actions CI workflow (fmt, clippy, test, build)
- [x] Add GitHub Release workflow with OS binaries
- [x] Auto-release on merged PRs to main based on release/{major|minor|patch} branch names
- [x] Auto-update daemon/CLI (daemon startup + daily, CLI fallback) with auditing and elevation prompts
- [x] Add CLI update command (check/apply)
- [x] Add TUI update flow (main menu + install view)
- [x] Update docs for self-update

## Milestone 43 — Installer Default Location + Update (in progress)

- [x] Install binary to OS default per-user location (app-data based)
- [x] Reinstall service using installed binary path
- [x] Add install manifest for detection/reporting
- [x] Update CLI/TUI messaging for install location
- [x] Update docs + acceptance notes for replace/update behavior
- [x] Use Windows Task Scheduler instead of Windows Service (fix 1053)
- [x] Distinguish install vs update flow in installer output
- [x] Show install status (version/time) inline on install page
- [x] Expand install status with task scheduler diagnostics

## Milestone 44 — Sync Status UX (in progress)

- [x] Add sync progress callbacks in core (per-repo current action)
- [x] Persist sync status + last action in cache
- [x] CLI sync `--status` progress bar with current action/repo
- [x] TUI Sync Status view with live status from cache

## Milestone 46 — Windows Task Scheduler UX (in progress)

- [x] Add `install --start` to run task after install
- [x] Add `task` subcommand (status/run/remove)
- [x] Expose task last run/result in install status

## Milestone 47 — Sync UX + Observability (in progress)

- [x] Add `sync --status-only` read-only command
- [x] Add `--audit-repo` toggle for per-repo audit entries
- [x] Show last error in Sync Status view

## Milestone 48 — Performance + Reliability (in progress)

- [x] Add `--jobs` to sync and daemon
- [x] Parallelize repo syncs safely per target

## Milestone 42 — Installer Single-Instance Fix (in progress)

- [x] Replace installer mutex with lockfile under data_local
- [x] Gate TUI installer entry with lock guard + release on exit
- [x] Remove redundant CLI lock acquisition before starting TUI install view
- [x] Drain pending input events when entering install view (prevents auto-install)
- [x] Ignore non-press key events in TUI to prevent double-handling
- [x] Add Windows admin check with clear error before service install
- [x] Add Unix/macOS permission preflight with sudo guidance when dirs aren't writable
- [x] Use Windows Service APIs for install (avoid sc.exe quoting issues)
- [x] Suppress external command output during TUI install (prevents UI corruption)
- [x] Remove net-session admin check to avoid console output in TUI
- [x] Add install progress output for non-interactive CLI
- [x] Add live install progress UI for TUI
- [x] Add install status view (TUI + CLI flag)
- [x] Extend install status with service running state
- [x] Add daemon sync audit records per target
- [x] Add per-repo audit records during sync (CLI + daemon)
- [x] Consider follow-up: update docs to mention installer lock behavior

## Milestone 38 — Build Fix (in progress)

- [x] Fix Windows build error in installer PATH registration
- [x] Fix Windows service start call type annotation
- [x] Re-run build to confirm

## Milestone 49 — TUI Provider Mgmt + Repo Overview (in progress)

- [x] Add provider-specific selection + hints in TUI target/token forms (GitHub/GitLab)
- [x] Add repo status cache + local status computation (branch/ahead/behind/last touched)
- [x] Add TUI repo overview tree view (folder structure with status columns)
- [x] Add tests for repo status + tree rendering helpers
- [x] Document assumptions and follow-ups

## Milestone 50 — TUI Sync Trigger (in progress)

- [x] Add dashboard hotkey to start sync (all targets)
- [x] Run sync in background with lockfile + audit
- [x] Surface completion/errors in TUI message view

## Milestone 39 — High-Severity Fixes (in progress)

- [x] Support GitHub user-scope targets (fallback from org endpoint)
- [x] Handle repo rename path moves safely
- [x] Harden repo name sanitization for Windows
- [x] Allow archive moves across devices

## Milestone 40 — Medium-Severity Fixes (in progress)

- [x] Drain retryable HTTP responses before retry
- [x] Add daemon backoff on repeated failures
- [x] Improve lockfile held detection on Windows

## Milestone 41 — Low-Severity Fixes (in progress)

- [x] Use stable lock file location under data_local
- [x] Avoid TUI target add audit relying on list tail

## Milestone 35 — TUI Main-Flow Guided UX (completed)

- [x] Add guided form hints + inline validation for Config Root, Targets, Tokens
- [x] Normalize main menu labels and footer help text for consistency
- [x] Apply minimal layout polish (headers, spacing, concise summaries)
- [x] Add tests for guidance rendering and validation text

## Milestone 36 — Dashboard (completed)

- [x] Add TUI dashboard view with core stats and per-target toggle
- [x] Update docs and acceptance checks for dashboard

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
- Installer lock: use data_local lockfile to enforce single installer across CLI/TUI.
- Repo overview: cache-only repo source, local git status only, 10-minute TTL refresh with manual override.
