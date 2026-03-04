# Skill: Self-Update and Release

## Goal

Ship safe binary updates with verification and rollback.

## Checklist

- Fetch signed release manifest.
- Verify checksum/signature before apply.
- Use atomic replacement strategy.
- Roll back automatically on failed replace.
- Emit traceable update events (start/success/failure/rollback).
- Keep channel behavior explicit (`stable`, optional `beta`).

## Must-Have Tests

- Happy-path update apply.
- Corrupt artifact verification failure.
- Rollback path after simulated replacement failure.

## Commit Pattern

1. Update manifest client + verification.
2. Atomic replace + rollback.
3. Release workflow and end-to-end tests.
