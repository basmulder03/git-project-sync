# Skill: CLI Parity

## Goal

Ensure every operational action is available via `syncctl`.

## Checklist

- Follow command contracts in `docs/CLI_SPEC.md`.
- Ensure daemon and one-shot sync actions both work.
- Add source/auth/workspace/cache/stats/trace commands as specified.
- Keep command output concise, script-friendly, and secret-safe.
- Keep error messages actionable with next-step hints.

## Must-Have Tests

- Command parsing and validation tests.
- Action parity tests between daemon API and CLI wrappers.
- Secret redaction tests for auth-related output.

## Commit Pattern

1. Core command groups.
2. Advanced ops commands.
3. CLI parity and output tests.
