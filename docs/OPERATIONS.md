# Operations Guide

## Service Modes

`git-project-sync` supports two service/registration modes:

- `user` mode:
  - Runs under the current user account.
  - Can only access repositories and credentials available to that user.
  - Preferred for developer workstations.
- `system` mode:
  - Runs with elevated/system-level privileges.
  - Requires explicit admin/root install steps.
  - Use when machine-wide scheduling or shared workspace access is required.

## Privilege Model

- Linux:
  - User mode writes service units to `~/.config/systemd/user`.
  - System mode writes service units to `/etc/systemd/system` and requires root.
- Windows:
  - User mode creates a Task Scheduler task scoped to the current user.
  - System mode creates a Task Scheduler task as `SYSTEM` and requires Administrator rights.

## Operational Safety Expectations

- Sync mutations are skipped on dirty repositories.
- Per-repository locking prevents concurrent mutating operations.
- Stale branch cleanup only deletes branches already merged and without unique commits.
- Event and trace history should be used for auditability of skipped/failed actions.

## Crash-Safe Recovery

- In-flight repo run metadata is persisted in the local state database.
- On daemon restart, unfinished runs are marked as `recovered` to avoid duplicate execution.
- Duplicate run IDs are rejected by recovery guardrails before scheduling work.
- Recovery outcomes remain queryable through run/event history.

## Provider Rate-Limit Handling

- Provider HTTP responses are inspected for throttling headers (`Retry-After`, `X-RateLimit-*`).
- When throttling is detected, scheduler applies adaptive per-source delay before the next attempt.
- Adaptive delays are source-scoped to avoid repeatedly hammering throttled provider accounts.
- Rate-limit waits are logged with reason code `provider_rate_limited`.

## Update Rollback Safety

- `syncctl update apply` performs checksum verification before replacement.
- Binary replacement is atomic rename-based with backup/rollback safeguards.
- Update events are emitted with reason codes:
  - `update_started`
  - `update_succeeded`
  - `update_failed`
  - `update_rollback` (when rollback is executed)

## Release Artifacts and Manifests

- Release workflow builds platform artifacts for Linux and Windows.
- `checksums.txt` is published for integrity auditing.
- `manifest.json` is published with channel/version metadata and artifact checksum entries.
- Updater clients consume the manifest URL and verify SHA-256 checksums before applying updates.

## Troubleshooting Permissions

- Linux `system` install fails with permission errors:
  - Re-run with `sudo`.
  - Confirm `systemctl` is available and systemd is active.
- Linux `user` service does not start:
  - Run `systemctl --user daemon-reload`.
  - Ensure the user session has systemd user services enabled.
- Windows task install fails:
  - Launch PowerShell as Administrator for `-Mode system`.
  - Confirm Task Scheduler service is running.
- Credential access fails after mode switch:
  - Re-run `syncctl auth login <source-id>` in the target user/system context.

## Install Preflight Diagnostics

- Run install-focused diagnostics with `syncctl doctor --install-mode user` or `syncctl doctor --install-mode system`.
- Doctor surfaces structured install findings as `finding: install_preflight reason_code=<code> severity=<level>`.
- For installer/registration failures, inspect reason code + hint and retry with corrected privileges/dependencies.

## Reason Code Troubleshooting Matrix

Use `syncctl events list`, `syncctl trace show <trace-id>`, `syncctl doctor`, and `syncctl stats show` as first-line diagnostics.

| Reason Code | Meaning | Immediate Operator Action |
| --- | --- | --- |
| `repo_conflicts` | Merge conflicts exist in working tree | Resolve conflicts manually, commit or abort merge, rerun sync |
| `repo_staged_changes` | Staged changes detected | Commit or unstage changes before allowing sync mutations |
| `repo_unstaged_changes` | Unstaged file modifications detected | Commit/discard local edits, then rerun sync |
| `repo_untracked_files` | Untracked files detected in repo | Add/ignore/move files and rerun sync |
| `upstream_missing` | Current branch has no upstream tracking ref | Set upstream (`git branch --set-upstream-to`) and rerun |
| `non_fast_forward` | Branch diverged from upstream | Perform manual reconciliation/rebase/merge and rerun |
| `repo_locked` | Another daemon run holds the repo lock | Wait for in-flight run to complete; verify lock clears in doctor output |
| `provider_rate_limited` | Provider throttling/backoff active | Wait for retry window; reduce parallelism if this repeats |
| `network_error` | Transient network/path failure | Check network/proxy/DNS and observe retry behavior |
| `timeout` | Operation exceeded timeout budget | Increase timeout if needed and inspect provider/network latency |
| `retry_budget_exceeded` | Retries exhausted within configured budget | Inspect root cause in trace, tune retry/timeout conservatively |
| `permanent_error` | Non-retryable provider/API/request failure | Fix payload/auth/config issue before rerunning |
| `update_failed` | Update apply failed | Check release artifact integrity, then inspect rollback outcome |
| `update_rollback` | Rollback executed after update failure | Confirm previous binary health, then retry update later |
| `install_unsupported_environment` | Installer invoked on unsupported OS/runtime | Run Linux installer on Linux or Windows installer on Windows |
| `install_invalid_mode` | Install mode value is invalid | Use `user` or `system` mode |
| `install_missing_binary_path` | Binary path was not configured | Set/override `BIN_PATH` to an existing syncd binary |
| `install_binary_missing` | syncd binary does not exist at expected path | Run bootstrap/download first, then retry registration |
| `install_dependency_missing` | Required platform tool missing (`systemctl`/`schtasks`) | Install/enable system service tooling and ensure it is on PATH |
| `install_insufficient_privileges` | System-mode install attempted without elevation | Re-run with `sudo`/Administrator shell or switch to user mode |
| `install_registration_failed` | Service/task registration command failed | Check command output, service manager health, and permissions |
| `install_validation_failed` | Preflight or post-registration validation failed | Resolve the cited preflight finding and retry install |
