# Architecture

## Suggested Stack

- Language: Go (single binary, Linux/Windows friendly, good CLI/service ecosystem).
- CLI: Cobra (or urfave/cli).
- TUI: Bubble Tea.
- Config: YAML + schema versioning.
- State store: local SQLite (or equivalent embedded DB) for non-sensitive runtime state.
- Token storage: OS keyring integration.

## High-Level Components

1. `syncd` daemon
   - Scheduler loop
   - Per-repo worker orchestration
   - Event/log persistence
   - Trace IDs for every sync run and repo job
   - Local control API for CLI/TUI
2. `syncctl` CLI
   - Full config management
   - Daemon control/actions
   - One-shot per-repo sync actions
3. `synctui`
   - Dashboard for status/stats/logs
   - Actions equivalent to CLI
4. Core library
   - Git safety engine
   - Provider adapters (GitHub/Azure)
   - Source/account registry (multi-account, multi-provider)
   - Workspace manager (clean repo folder layout)
   - Cache manager
   - State persistence layer (config + local DB)
   - Update manager
   - Installer/service registration logic

## Source and Account Model

- A `source` is a provider account binding (for example one GitHub PAT or one Azure DevOps PAT).
- Multiple sources can be active at once across providers.
- Sources support personal/private and corporate/team/organization account contexts.
- Repositories map to a specific source ID.

## Workspace Layout (Managed)

Use a deterministic workspace path convention for easy navigation:

`<workspace_root>/<provider>/<account_or_org>/<project_or_repo>`

Examples:

- `workspace/github/acme-org/platform-api`
- `workspace/github/jane-doe/dotfiles`
- `workspace/azuredevops/contoso/erp-service`

## Repository Layout (Target)

```text
cmd/
  syncd/
  syncctl/
  synctui/
internal/
  core/
    sync/
    git/
    providers/
    auth/
    config/
    cache/
    daemon/
    update/
    install/
    telemetry/
docs/
ai/
scripts/
```

## Sync Flow (Per Repo)

1. Acquire repo lock.
2. Inspect dirty state (staged/unstaged/untracked/conflicts).
3. If dirty: log skip reason and exit.
4. Fetch remote refs and prune stale refs.
5. Resolve default branch (`origin/HEAD` -> provider API fallback).
6. Compare current branch to upstream.
7. Fast-forward update current branch when safe and behind.
8. Ensure default branch local ref is up to date.
9. Evaluate stale-branch cleanup conditions.
10. If checked-out branch is stale and safe to delete:
    - checkout default branch
    - delete stale local branch
11. Emit result event + metrics.
12. Release lock.

## Local Control API

Expose daemon functions to CLI/TUI via local IPC (named pipe / unix socket):

- Get daemon status
- Trigger sync all
- Trigger sync single repo
- List repositories and state
- Show/refresh cache
- Show logs/events
- Show trace details by run ID
- Reload config

## Update Strategy

- Signed release manifest
- Checksum verification
- Atomic binary swap
- Rollback on failed replace
- Channels: `stable` (v1), optional `beta`
