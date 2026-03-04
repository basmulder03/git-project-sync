# Implementation Plan

## Phase 1 - Core Safety + Sync Engine

- Build git wrapper and dirty-state checks.
- Implement structured logging, event model, and trace/run IDs from day one.
- Resolve default branch from remote HEAD.
- Implement fast-forward-only update logic.
- Implement stale branch cleanup guardrails.
- Implement config + local state persistence foundation.
- Implement managed workspace layout resolver.

Exit criteria:
- Acceptance tests 1, 3, 4, 5, 8, 17, 18, 21, 22 pass.

## Phase 2 - Provider Integrations

- Add GitHub and Azure DevOps provider adapters.
- Validate PAT tokens and provider connectivity.
- Implement secure token storage abstraction.
- Implement multi-source registry supporting multiple accounts per provider.
- Validate personal/private and org/team account contexts.

Exit criteria:
- Acceptance tests 13, 14, 19, and 20 pass.

## Phase 3 - Daemon and IPC

- Add periodic scheduler with retries/jitter.
- Add per-repo lock manager.
- Add local control API used by CLI/TUI.

Exit criteria:
- Stable periodic sync behavior with concurrent safety.

## Phase 4 - CLI Parity

- Implement full CLI command set from `docs/CLI_SPEC.md`.
- Ensure daemon operations and one-shot actions work.

Exit criteria:
- Acceptance test 12 pass.

## Phase 5 - Lightweight TUI

- Add dashboard, repo list, logs/events, cache views.
- Add manual action triggers.

Exit criteria:
- TUI functional parity for major operational actions.

## Phase 6 - Install + Service Registration

- Linux user/system registration flow.
- Windows registration flow with privilege handling.

Exit criteria:
- Acceptance tests 10 and 11 pass.

## Phase 7 - Self-Update + Release

- Implement signed manifest + checksums verification.
- Add update apply with rollback path.

Exit criteria:
- Acceptance tests 15 and 16 pass.

## Phase 8 - Hardening + Cross-Platform CI

- Add integration tests and matrix CI for Linux/Windows.
- Complete docs and operational guidance.

Exit criteria:
- Full acceptance suite green on CI.
