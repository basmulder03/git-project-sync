# Skill: Platform Install and Service

## Goal

Deliver reliable Linux and Windows registration/install flows.

## Checklist

- Support Linux user/system install modes.
- Support Windows registration mode (Task Scheduler in v1).
- Ensure install/uninstall scripts are idempotent.
- Validate privilege requirements and clear error messages.
- Document permission model and troubleshooting.

## Must-Have Tests

- Linux registration lifecycle (register/start/stop/unregister).
- Windows registration lifecycle validation.
- Script dry-run/error-path tests.

## Commit Pattern

1. Linux implementation.
2. Windows implementation.
3. Docs + integration tests.
