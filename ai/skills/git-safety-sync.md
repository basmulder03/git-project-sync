# Skill: Git Safety Sync

## Goal

Implement sync behavior without risking user data.

## Checklist

- Verify dirty state (staged, unstaged, untracked, conflicts) before mutation.
- Use fast-forward-only update behavior.
- Resolve default branch dynamically (`origin/HEAD`, provider fallback).
- Never use destructive commands (`reset --hard`, force delete, force checkout).
- Before local branch deletion, verify no unique local commits.
- Log explicit skip reason codes for all blocked actions.

## Must-Have Tests

- Dirty repo is skipped.
- Non-fast-forward divergence is skipped safely.
- Merged stale branch deletes only when safe.

## Commit Pattern

1. Sync decision logic.
2. Safety guards and deletion checks.
3. Tests for safety paths.
