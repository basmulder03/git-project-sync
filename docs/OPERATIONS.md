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
