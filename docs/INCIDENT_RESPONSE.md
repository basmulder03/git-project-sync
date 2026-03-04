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

## Post-Incident Checklist

- Capture trace IDs and top reason codes from the incident window.
- Document operator actions taken and resulting state.
- Add follow-up items for config tuning, token rotation, or docs updates.
- If safety guards triggered (`repo_*`, `non_fast_forward`), keep manual intervention audit notes.
