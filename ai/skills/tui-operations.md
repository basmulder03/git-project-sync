# Skill: TUI Operations

## Goal

Provide lightweight, practical operational visibility and controls.

## Checklist

- Show daemon state, next run, and active/failed jobs.
- Show per-repo health (dirty, ahead/behind, last result).
- Show cache age and refresh actions.
- Show event timeline with trace IDs.
- Keep keyboard navigation simple and responsive.
- Ensure every TUI action has CLI equivalent.

## Must-Have Tests

- View state mapping tests.
- Action dispatch tests against daemon control API.
- TUI/CLI parity checks for key operations.

## Commit Pattern

1. App shell and dashboard.
2. Repo/cache/log panels.
3. Action wiring and parity tests.
