# Troubleshooting Guide

Use this guide for common failures and fast remediation.

## First Checks

Run these commands first:

```bash
syncctl doctor
syncctl stats show
syncctl events list --limit 100
```

If needed, drill into a single trace:

```bash
syncctl trace show <trace-id>
```

## Common Symptoms

## Repositories Not Updating

- Check for dirty state reason codes:
  - `repo_staged_changes`
  - `repo_unstaged_changes`
  - `repo_untracked_files`
  - `repo_conflicts`
- Resolve local git state, then rerun sync.

## Frequent `repo_locked` Events

- Confirm only one daemon instance is active.
- Reduce concurrency:
  - `daemon.max_parallel_repos`
  - `daemon.max_parallel_per_source`
- See `docs/operations/incident-response-playbook.md` for lock-contention runbook.

## Network or Provider Failures

- Reason codes to watch:
  - `network_error`
  - `timeout`
  - `provider_rate_limited`
- Validate DNS/proxy/firewall and provider status pages.

## Installer Failures

- Use installer diagnostics:

```bash
syncctl doctor --install-mode user
syncctl doctor --install-mode system
```

- Common install reason codes:
  - `install_dependency_missing`
  - `install_insufficient_privileges`
  - `install_registration_failed`
  - `install_validation_failed`

## State DB Issues

- Backup, validate, and restore:

```bash
syncctl state backup --output /tmp/sync.db.backup
syncctl state check
syncctl state restore --input /tmp/sync.db.backup
```

## Update Failures

- Check update reason codes:
  - `update_failed`
  - `update_rollback`
- Validate release artifacts/checksums and retry once root cause is fixed.

## Escalation

- Follow severity matrix and response targets in `docs/operations/incident-response-playbook.md`.
- If SLO risk persists, follow `docs/operations/reliability-slos-and-error-budgets.md` error budget policy.
