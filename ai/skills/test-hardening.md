# Skill: Test Hardening

## Goal

Prove behavior under realistic and failure-heavy conditions.

## Checklist

- Add integration tests for all safety-critical flows.
- Cover multi-source credential routing and traceability behavior.
- Add Linux/Windows CI matrix.
- Add failure-injection tests (network issues, throttling, restart recovery).
- Add long-run soak checks for scheduler stability.

## Must-Have Tests

- Dirty-repo skip behavior.
- Stale-branch deletion safety.
- Multi-source account coexistence.
- Installer/update regression tests.

## Commit Pattern

1. Integration coverage for core safety.
2. Platform/update/reliability coverage.
3. CI matrix and test documentation updates.
