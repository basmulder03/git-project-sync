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

Current Sprint-1 integration coverage:

- Dirty repo safety skip behavior with explicit reason code and trace linkage.
- Multi-source scheduler routing and trace event persistence/query behavior.
- CI workflow runs `go test ./...` and `go test ./tests/integration/...` on Linux and Windows.

Exit criteria:
- Full acceptance suite green on CI.

## Phase 9 - Release Readiness and Install Bootstrap

- Add first-install bootstrap for fresh machines without preinstalled CLI.
- Add install preflight diagnostics and actionable failure hints.
- Add coverage inventory, thresholds, and CI reporting.

Exit criteria:
- Fresh machine can install and configure via bootstrap flow on Linux/Windows.
- Coverage thresholds are enforced for critical packages.

## Phase 10 - Scale and Operational Excellence

- Add large-scale scheduler tests and restart/lock contention drills.
- Add state DB backup/restore and integrity tooling.
- Define and document operational SLOs and severity matrix.

Exit criteria:
- Scale/resilience reliability suite passes consistently.
- Operators can triage and recover state with documented procedures.

## Phase 11 - UX Polish and v1.0 Launch

- Final CLI/TUI UX consistency pass.
- Improve first-run onboarding and install docs.
- Add release-candidate checklist mapped to acceptance criteria.

Exit criteria:
- v1.0 release candidate gates are documented and passing.

## Phase 12 - Enterprise Governance

- Add policy controls for allowed sync behavior and protected repos.
- Add audit-friendly export workflows for traces/events.
- Extend diagnostics with policy drift and governance checks.

Exit criteria:
- Governance controls are enforceable and auditable.

## Phase 13 - Ecosystem Integrations

- Add notification sinks for critical events.
- Add maintenance windows/blackout scheduling controls.
- Add export modes for external observability systems.

Exit criteria:
- Integration automation is safe, redacted, and operationally useful.

## Phase 14 - Performance and LTS Baseline

- Add benchmark suite and regression gates.
- Reduce integration flakiness and document triage process.
- Define long-term support lifecycle and maintenance policy.

Exit criteria:
- Performance and reliability baselines are enforced in CI.
- LTS policy and maintenance workflow are published.
