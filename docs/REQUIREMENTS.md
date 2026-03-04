# Requirements

## Product Goal

Keep many local Git repositories safely synchronized with their remote default branch using a background service, with full control through CLI and TUI.

## Functional Requirements

1. Detect provider type per repository (GitHub/Azure DevOps for v1).
2. Authenticate provider operations via PAT tokens.
3. Support multiple provider sources at the same time (multiple GitHub and Azure DevOps accounts).
4. Support both personal/private and corporate/team/organization account contexts.
5. Resolve remote default branch dynamically (`main`, `master`, or other).
6. Periodically fetch and update local refs in a background daemon.
7. Update current branch when behind its upstream branch using fast-forward-only behavior.
8. Update default branch when behind remote.
9. Skip all mutating operations when repository is dirty.
10. Detect stale local branch after it is merged into default branch.
11. If stale branch is currently checked out: switch to default branch safely, then delete stale local branch.
12. Before any branch deletion, verify no unique local commits would be lost.
13. Manage repositories in a clean, navigable workspace layout.
14. Provide CLI commands for all daemon actions and all config operations.
15. Provide lightweight TUI dashboard for status, actions, logs, and cache controls.
16. Support cache visibility and force-refresh actions for relevant data.
17. Run on Linux and Windows.
18. Support self-update for distributed binaries.
19. Provide installer/registration flow that requests required privileges.

## Safety Requirements

- Never use destructive sync operations (`reset --hard`, force checkout, forced branch delete).
- Never auto-stash user changes.
- Never run sync if worktree/index is dirty.
- Never delete local branches with unpublished commits.
- Always log explicit reason when a repo/action is skipped.
- Always use per-repository locking to avoid concurrent mutations.

## Operational Requirements

- Logging and traceability are mandatory from phase 1.
- Structured logs must include correlation IDs/job IDs and skip/action reasons.
- Action/event history must be queryable from CLI and visible in TUI.
- Configurable sync interval with jitter and retry backoff.
- Timeouts for provider and git operations.
- Health/status endpoint or local control API for CLI/TUI.
- Dry-run mode for diagnostics.
- Persist non-sensitive configuration and runtime state in config file and/or local database.

## Security Requirements

- Store PAT tokens only in OS-backed secure credential storage when available.
- Provide secure fallback only when OS credential storage is unavailable.
- Never write tokens to logs.
- Redact secrets from command output and telemetry.
- Support token validation and rotation flow.

## Non-Goals (v1)

- Auto-merge/rebase workflows.
- Forceful conflict resolution.
- Pull request management.
- Auto-push local commits.
