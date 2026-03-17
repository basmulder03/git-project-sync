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
  max_parallel_per_source: 2
  operation_timeout_seconds: 120
  retry:
    max_attempts: 3
    base_backoff_seconds: 2
  maintenance_windows:
    - name: nightly-deploy
      days: [monday, tuesday, wednesday, thursday, friday]
      start: "02:00"
      end: "04:00"
    - name: weekend-maintenance
      days: [saturday, sunday]
      start: "00:00"
      end: "23:59"

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

governance:
  default_policy:
    include_repo_patterns:
      - '^/workspace/'
    exclude_repo_patterns:
      - '/archive/'
    protected_repo_patterns:
      - '/critical/'
    allowed_sync_windows:
      - days: [monday, tuesday, wednesday, thursday, friday]
        start: "08:00"
        end: "20:00"
  source_policies:
    gh-work:
      protected_repo_patterns:
        - '/regulated/'
      allowed_sync_windows:
        - days: [monday, wednesday, friday]
          start: "09:00"
          end: "17:00"

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

notifications:
  sinks:
    - name: ops-webhook
      type: webhook
      url: https://hooks.example.com/git-sync
      min_severity: error
      reason_codes: []        # empty = all reasons; list specific codes to filter
      enabled: true
    - name: slack-alerts
      type: slack
      url: https://hooks.slack.com/services/T000/B000/xxxx
      min_severity: warn
      reason_codes:
        - sync_failed
        - maintenance_window_active
      enabled: true
    - name: teams-channel
      type: teams
      url: https://outlook.office.com/webhook/...
      min_severity: error
      enabled: false          # disabled without removing the config
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
- Scheduler dispatch is fair across sources (round-robin) to prevent source starvation.
- `daemon.max_parallel_repos` limits total concurrent repo sync jobs.
- `daemon.max_parallel_per_source` limits concurrent repo sync jobs per source/account.
- Governance policies can block sync by include/exclude/protected patterns and allowed time windows.
- Policy blocks emit explicit reason codes (`policy_repo_not_included`, `policy_repo_excluded`, `policy_repo_protected`, `policy_outside_sync_window`).
- `daemon.maintenance_windows` defines periods during which ALL mutating sync operations are suppressed.
  - `name` — human-readable label used in logs and telemetry.
  - `days` — list of weekday names (e.g. `monday`, `tue`, `saturday`). Short and long forms accepted.
  - `start` / `end` — 24-hour clock in `HH:MM` format. End is exclusive (a window of `02:00`–`03:00` ends at 03:00:00).
  - Emits telemetry events with `reason_code: maintenance_window_active` for every skipped task.
  - Use `syncctl maintenance status` and `syncctl maintenance list` to inspect active/configured windows.
- `notifications.sinks` configures outbound event delivery to webhooks, Slack incoming webhooks, or MS Teams connectors.
  - `type` — one of `webhook`, `slack`, `teams`.
  - `min_severity` — minimum event level to deliver: `info`, `warn`, or `error` (default `error`).
  - `reason_codes` — optional allowlist of telemetry reason codes. Empty means all reason codes pass.
  - `enabled` — set to `false` to temporarily silence a sink without deleting its configuration.
  - Payloads are always redacted: no PAT tokens, no raw file-system paths, no credentials.
  - HTTP failures are logged but never propagate errors or panic the daemon.
