# User Guide

This guide explains day-to-day usage of `git-project-sync` for operators and developers.

## Typical Workflow

1. Install and register services (`docs/INSTALL.md`).
2. Complete first-run onboarding (`docs/QUICKSTART.md`).
3. Monitor health and event history.
4. Use TUI/CLI actions for triage and manual interventions.

## Core Commands

Check service health:

```bash
syncctl doctor
syncctl stats show
syncctl events list --limit 50
```

Inspect one run:

```bash
syncctl trace show <trace-id>
```

Run sync manually:

```bash
syncctl sync all --dry-run
syncctl sync all
```

## Safety Model in Practice

- Dirty repositories are skipped instead of mutated.
- Non-fast-forward and unsafe branch states are skipped with reason codes.
- Concurrent operations on the same repository are lock-protected.

## TUI Usage

- `synctui`: runtime dashboard for status, repos, cache, logs, and triage.
- `syncsetup`: setup/repair dashboard for install, update, downgrade confirmation, and reinstall.

## State and Recovery

Back up local state DB:

```bash
syncctl state backup --output /path/to/backup.db
```

Integrity check:

```bash
syncctl state check
```

Restore backup:

```bash
syncctl state restore --input /path/to/backup.db
```

## Where to Go Next

- Operational procedures: `docs/OPERATIONS.md`
- Incident handling: `docs/INCIDENT_RESPONSE.md`
- SLOs and reliability targets: `docs/SLOS.md`
