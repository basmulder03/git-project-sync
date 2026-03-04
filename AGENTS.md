# Git Project Sync - Agent Playbook

This repository is prepared for autonomous implementation by coding agents (Codex, Claude Code, etc.).

## Objective

Build a cross-platform application that keeps local git repositories in sync with their remote default branch (`main` or `master`) while preserving local safety.

## Hard Requirements

- OS support: Linux and Windows.
- Authentication: PAT tokens for GitHub and Azure DevOps.
- Support multiple provider sources at the same time (multiple GitHub and Azure DevOps accounts).
- Support both personal/private and corporate/team/organization account contexts.
- Background daemon/service for periodic sync.
- CLI for full configuration and all daemon actions.
- Lightweight TUI or GUI (TUI recommended for v1).
- Logging and traceability are mandatory from the first implementation phase.
- Never modify repos when working tree is dirty.
- Safe stale-branch cleanup after merge to default branch.
- Auto-update support for distributed binaries.
- Installer/registration flow that requests required privileges.
- Managed repository workspace layout that is clean and easily navigable.
- Persist all non-sensitive state/configuration in config file and/or local database.
- Persist sensitive data only in OS credential manager (or secure equivalent fallback).
- Clear PAT permission documentation.

## Safety Rules (Do Not Violate)

- Never use destructive git operations (`reset --hard`, forced branch deletion, force checkout) in automation paths.
- Never delete a local branch if it has commits not present on its upstream/default branch.
- Never auto-stash user changes.
- Never run sync operations on dirty repositories.
- Always log why an action was skipped.

## Delivery Order

1. Core git safety + sync engine.
2. Provider integration (GitHub/Azure DevOps).
3. Background daemon.
4. CLI parity with daemon actions.
5. TUI dashboard.
6. Installers/service registration.
7. Self-update.
8. Hardening/tests/docs.

## Source of Truth

Use these files together:

- `docs/REQUIREMENTS.md`
- `docs/ARCHITECTURE.md`
- `docs/CLI_SPEC.md`
- `docs/PAT_PERMISSIONS.md`
- `docs/ACCEPTANCE_TESTS.md`
- `ai/agents.yaml`
- `ai/backlog.yaml`
