# Service Level Objectives (SLOs)

This document defines operator-facing reliability objectives for `git-project-sync`.

## Scope

- Applies to daemon-driven synchronization in `user` and `system` service modes.
- SLO compliance is evaluated over a rolling 30-day window unless noted otherwise.

## Core SLOs

### SLO-1: Sync Freshness

- Objective: 99% of enabled repositories are synchronized within 2 daemon intervals after the latest remote default-branch update is detectable.
- Measurement source: daemon run history + event timestamps (`sync_completed`, `repo_locked`, retry reasons).
- Alert threshold: breach risk if >2% of repos exceed freshness target for 3 consecutive cycles.

### SLO-2: Sync Success Rate

- Objective: >= 99.5% of attempted repo sync jobs complete without terminal error in each 24-hour period.
- Excludes policy skips that are safety-preserving (`repo_staged_changes`, `repo_unstaged_changes`, `repo_untracked_files`, `repo_conflicts`).
- Alert threshold: terminal failures (`permanent_error`, `retry_budget_exceeded`, `update_failed`) >0.5% of attempts.

### SLO-3: Daemon Recovery Time

- Objective: 95% of daemon restarts recover in-flight state and resume scheduling within 5 minutes.
- Measurement source: restart timestamp, subsequent recovered run markers, and next successful cycle completion.
- Alert threshold: recovery >5 minutes for two or more consecutive restarts.

### SLO-4: Update Safety

- Objective: 100% of update applies are checksum-validated before replacement; 100% of failed applies either rollback cleanly or preserve prior binary.
- Measurement source: update events (`update_started`, `update_succeeded`, `update_failed`, `update_rollback`) and release artifact checksums.

## Error Budget Policy

- Freshness error budget: 1% of repo-cycle opportunities per 30 days.
- Sync success error budget: 0.5% of attempted jobs per 24 hours.
- Recovery error budget: 5% of restart events per 30 days.
- If any error budget is exhausted:
  - freeze non-essential feature rollout,
  - prioritize reliability fixes and runbook hardening,
  - require explicit incident owner sign-off before resuming normal rollout pace.

## Severity Mapping Overview

Detailed severity matrix is maintained in `docs/operations/incident-response-playbook.md`.

- Sev-1: sustained global service unavailability or data safety risk.
- Sev-2: major degradation affecting multiple sources/repos or SLO breach in progress.
- Sev-3: localized degradation with workaround available.
- Sev-4: low-impact operational defect or informational anomaly.
