# User Guide

This guide explains day-to-day usage of `git-project-sync` for operators and developers.

## Typical Workflow

1. Install and register services (`docs/getting-started/installation-and-service-registration.md`).
2. Complete first-run onboarding (`docs/getting-started/first-run-onboarding.md`).
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

### Dashboard Controls

- `h` / `left`, `l` / `right`: switch tabs.
- `tab`: switch focused status panel (overview vs recent errors).
- `r`: refresh now.
- `s`: run `sync all`.
- `c`: run `cache refresh all`.
- `t`: run trace drill-down for latest trace.
- `/`: open command palette.
- `!`: re-run the last palette command.

### Command Palette

- The palette is searchable and supports:
  - `up`/`down` (or `j`/`k`) to select suggestions.
  - `tab` to autocomplete the selected suggestion.
  - `enter` to execute.
- Palette parity with CLI top-level groups is tracked in `docs/reference/cli-tui-parity-matrix.yaml`.

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

- Operational procedures: `docs/operations/service-operations-guide.md`
- Incident handling: `docs/operations/incident-response-playbook.md`
- SLOs and reliability targets: `docs/operations/reliability-slos-and-error-budgets.md`
