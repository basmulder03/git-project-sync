# Acceptance Tests

## Safety

1. Dirty repo is never mutated
   - Given staged/unstaged/untracked changes
   - When daemon sync runs
   - Then no checkout/pull/delete occurs
   - And skip reason is logged

2. No destructive git commands
   - Verify sync paths do not invoke reset/force-delete/force-checkout operations.

3. Branch deletion protection
   - Given local branch has commits not on upstream/default
   - When stale cleanup is evaluated
   - Then branch is not deleted and reason is logged

## Sync Behavior

4. Default branch discovery
   - Repo default branch is `main` or `master` (or custom)
   - Tool resolves it correctly each run

5. Current branch fast-forward
   - Given current branch behind upstream and clean worktree
   - Tool fast-forwards successfully

6. Default branch update
   - Given local default branch behind remote
   - Tool updates default branch local ref safely

7. Non-fast-forward situation
   - Given divergence/conflict
   - Tool skips mutation and logs clear remediation hint

## Stale Branch Cleanup

8. Checked-out stale branch cleanup
   - Given checked-out feature branch merged into default
   - And no unique commits exist
   - Tool switches to default and deletes local stale branch
   - Tool also deletes other local branches that are already merged and safe
   - If branch still has unique commits, cleanup is skipped with explicit reason code

9. Not merged branch preserved
   - Given branch not merged into default
   - Tool never deletes branch

## Platform and Service

10. Linux service registration
    - Install/register daemon for user and system modes
    - Start/stop/status actions work

11. Windows service/task registration
    - Install/register in supported mode
    - Start/stop/status actions work

12. CLI parity
    - Every TUI action has a CLI equivalent
    - CLI can perform all daemon actions

## Auth and Security

13. PAT login and validation
    - Valid PAT succeeds for GitHub and Azure
    - Invalid PAT rejected with actionable error

14. Secret redaction
    - Tokens are never shown in logs/events/TUI

## Update

15. Self-update success path
    - Tool checks and applies update with checksum validation

16. Self-update rollback path
    - Simulated failed replace recovers prior executable safely

## Logging and Traceability

17. Traceability from day one
    - Every daemon sync run has a trace/run ID
    - Repo-level actions are linked to the same trace/run ID
    - CLI can query trace details

18. Skip/action reason logging
    - Every skipped mutation includes explicit reason code and human-readable reason
    - TUI/CLI can list recent events with timestamps

## Multi-Source Accounts

19. Multiple active sources
    - Configure at least two GitHub sources and two Azure sources concurrently
    - Sync operations resolve the correct source credentials per repository

20. Personal and organization contexts
    - Repositories from personal/private accounts and org/team accounts both sync successfully

## Workspace and Persistence

21. Managed workspace layout
    - Repositories are placed under deterministic provider/account/repo paths
    - CLI can validate and report layout drift

22. Persistence separation
    - Non-sensitive data persists in config file and/or local DB
    - Sensitive data exists only in OS credential manager (or secure fallback)
