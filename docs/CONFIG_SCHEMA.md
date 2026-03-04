# Config Schema (v1)

```yaml
schema_version: 1

workspace:
  root: /path/to/workspace
  layout: provider-account-repo
  create_missing_paths: true

state:
  db_path: /path/to/state.db
  persist_events_days: 30

daemon:
  interval_seconds: 300
  jitter_seconds: 30
  max_parallel_repos: 4
  operation_timeout_seconds: 120
  retry:
    max_attempts: 3
    base_backoff_seconds: 2

update:
  channel: stable
  auto_check: true
  auto_apply: false

logging:
  level: info
  format: json
  redact_secrets: true

cache:
  provider_ttl_seconds: 900
  branch_ttl_seconds: 120

sources:
  - id: gh-personal
    provider: github
    account: jane-doe
    organization: ""
    host: github.com
    enabled: true
    credential_ref: keyring://git-project-sync/sources/gh-personal
  - id: gh-work
    provider: github
    account: jane-work
    organization: acme-org
    host: github.com
    enabled: true
    credential_ref: keyring://git-project-sync/sources/gh-work
  - id: az-work
    provider: azuredevops
    account: contoso
    organization: platform-team
    host: dev.azure.com
    enabled: true
    credential_ref: keyring://git-project-sync/sources/az-work

repos:
  - path: /path/to/repo
    source_id: gh-personal
    remote: origin
    enabled: true
    provider: auto
    cleanup_merged_local_branches: true
    skip_if_dirty: true
```

## Notes

- PAT tokens are not stored in this file.
- Provider credentials should be in OS keyring/credential manager with source-specific keys.
- Non-sensitive runtime state can be persisted in local DB and/or config file.
- State DB schema is migration-versioned and managed independently from `schema_version`.
- Recommended runtime persistence split:
  - Config file: static, non-sensitive settings (`sources`, `repos`, daemon/update/logging/cache settings)
  - SQLite state DB: mutable runtime state (repo sync snapshots, event history, traces)
- Path handling must support both Linux and Windows separators.
- Workspace layout should be deterministic and easy to navigate.
