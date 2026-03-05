# Incident Response Playbook

This runbook defines safe triage and recovery procedures for common production incidents.

## Incident Triage Flow

1. Confirm daemon status:
   - `syncctl daemon status`
2. Collect health and event signals:
   - `syncctl doctor`
   - `syncctl stats show`
   - `syncctl events list --limit 100`
3. Drill into one failing run:
   - `syncctl trace show <trace-id>`
4. Identify reason code and use the matching procedure below.

## Auth Failure Recovery

Symptoms:
- Repeated `permanent_error` events tied to a provider source.
- `syncctl doctor` indicates auth/credential issues.

Procedure:
1. Verify source configuration (`syncctl source list` / config file).
2. Validate current token: `syncctl auth test <source-id>`.
3. Re-authenticate if needed: `syncctl auth login <source-id>`.
4. Rerun targeted sync: `syncctl repo sync <path> --dry-run`.
5. Resume daemon cycle after successful validation.

## Lock Contention Recovery

Symptoms:
- Frequent `repo_locked` events for the same repositories.

Procedure:
1. Confirm only one daemon instance is active.
2. Check recent traces for long-running operations.
3. Tune concurrency if needed:
   - lower `daemon.max_parallel_repos`
   - lower `daemon.max_parallel_per_source`
4. Restart daemon if lock state appears stale.
5. Verify lock contention returns to normal levels in `syncctl stats show`.

## Restart Storm Recovery

Symptoms:
- Frequent daemon restarts while sync work is active.
- Repeated short-lived in-flight runs with degraded throughput.

Procedure:
1. Stabilize process lifecycle (disable crash loop trigger, stop external restarts temporarily).
2. Confirm in-flight recovery behavior after restart:
   - `syncctl doctor`
   - `syncctl events list --limit 100`
3. Verify no stale in-flight state remains and new runs progress normally.
4. Resume normal daemon restart policy once run completion and lock behavior normalize.

Escalation criteria:
- If repeated restart storms continue for more than 2 scheduler intervals, escalate as a service reliability incident.
- If lock contention and restart storms occur together, reduce concurrency and treat as sev-2 until stable.

## Update Failure and Rollback Recovery

Symptoms:
- `update_failed` followed by `update_rollback`.

Procedure:
1. Confirm the service is healthy on the rolled-back version.
2. Check release manifest/checksum availability and integrity.
3. Inspect update trace events for exact failure phase.
4. Retry update once root cause is fixed:
   - `syncctl update check`
   - `syncctl update apply --channel stable`
5. If repeated failures persist, pin current version and open an incident.

## Network and Provider Throttling Recovery

Symptoms:
- Repeated `network_error`, `timeout`, or `provider_rate_limited`.

Procedure:
1. Validate outbound connectivity, proxy, and DNS.
2. Confirm provider status page and account quota limits.
3. Reduce pressure by tuning daemon concurrency and retry settings.
4. Re-run `syncctl doctor` and monitor event trends for 1-2 cycles.

## Severity Matrix and Response Timelines

| Severity | Typical Conditions | Initial Response Target | Escalation Target |
| --- | --- | --- | --- |
| `sev-1` | Service-wide outage, repeated crash loops without recovery, or safety guarantees at risk | 15 minutes | 30 minutes |
| `sev-2` | Multi-source sync degradation, restart storm with rising backlog, or active SLO breach risk | 30 minutes | 60 minutes |
| `sev-3` | Localized source/repo failures, persistent lock contention, degraded but operating service | 4 hours | Next business window |
| `sev-4` | Low-impact anomalies, intermittent warnings, or non-critical operator friction | 1 business day | Backlog grooming |

### Reason-Code to Severity Defaults

| Reason Code / Class | Default Severity | Notes |
| --- | --- | --- |
| `repo_conflicts`, `repo_staged_changes`, `repo_unstaged_changes`, `repo_untracked_files` | `sev-4` | Safety-preserving skips; user action usually required |
| `repo_locked` | `sev-3` | Raise to `sev-2` if sustained across many repos or accompanied by restart storm |
| `provider_rate_limited`, `network_error`, `timeout` | `sev-3` | Raise to `sev-2` when widespread and affecting freshness SLO |
| `retry_budget_exceeded`, `permanent_error` | `sev-2` | Can become `sev-1` if global and prolonged |
| `update_failed`, `update_rollback` | `sev-2` | Raise to `sev-1` if rollback fails or binary health is degraded |
| `install_*` failures in production rollout | `sev-3` | Raise based on blast radius (fleet-wide => `sev-2`) |

## Post-Incident Checklist

- Capture trace IDs and top reason codes from the incident window.
- Document operator actions taken and resulting state.
- Add follow-up items for config tuning, token rotation, or docs updates.
- If safety guards triggered (`repo_*`, `non_fast_forward`), keep manual intervention audit notes.
