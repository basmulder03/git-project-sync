# Auto-Discovery and Auto-Clone

## Overview

The auto-discovery feature automatically queries provider APIs (GitHub, Azure DevOps) to discover accessible repositories and clone missing ones into your workspace. This ensures your local workspace stays in sync with your organization's repositories without manual intervention.

## Key Features

- **Automatic Discovery**: Queries provider APIs for all accessible repositories
- **Governance Filtering**: Applies include/exclude patterns from your configuration
- **Size Limits**: Respects maximum repository size constraints
- **Archived Handling**: Optionally includes or excludes archived repositories
- **Sequential Cloning**: Clones repositories one at a time to avoid overwhelming disk/network
- **Non-Blocking**: Discovery runs in background, sync operations continue normally
- **Retry Logic**: Smart retry for transient errors, logging for permanent failures
- **State Tracking**: All discoveries and clone operations logged to state database

## Configuration

### Enable Auto-Clone

In your `config.yaml`:

```yaml
governance:
  default_policy:
    # Enable auto-discovery and cloning
    auto_clone_enabled: true
    
    # Skip repos larger than 2GB (0 = unlimited)
    auto_clone_max_size_mb: 2048
    
    # Whether to clone archived repositories
    auto_clone_include_archived: false
    
    # Filter which repos to clone
    include_repo_patterns:
      - "^myorg/.*"
      - "^important-.*"
    
    exclude_repo_patterns:
      - ".*-archive$"
      - "^test-.*"
```

### Discovery Scheduling

Configure how often discovery runs:

```yaml
daemon:
  interval_seconds: 300              # Sync repos every 5 minutes
  discovery_interval_seconds: 3600   # Discover new repos every hour
```

**Note**: Discovery runs much less frequently than sync operations to minimize API calls.

Setting `discovery_interval_seconds: 0` disables periodic discovery after the initial run at daemon startup.

## Usage

### Automatic Discovery (Daemon Mode)

When running `syncd` in daemon mode:

1. Discovery runs immediately at startup (in background)
2. Discovery repeats at configured intervals (default: 1 hour)
3. Sync operations continue normally during discovery
4. Newly discovered repos are included in subsequent sync cycles

```bash
# Start daemon with auto-discovery
syncd

# Discovery logs appear as:
# level=info msg="starting background discovery" trace_id=discovery-1234567890
# level=info msg="background discovery completed" trace_id=discovery-1234567890
```

### Manual Discovery (CLI)

Trigger discovery on-demand:

```bash
# Run discovery now
syncctl discover

# Preview what would be discovered (dry-run)
syncctl discover --dry-run
```

The command outputs a trace ID for monitoring:

```
starting discovery (trace_id=discover-1234567890)
discovery completed successfully
check events with: syncctl trace show discover-1234567890
```

### One-Time Discovery (Once Mode)

Run discovery once and exit:

```bash
# Discover, clone, sync, then exit
syncd --once
```

**Note**: In `--once` mode, discovery runs synchronously before sync to ensure newly cloned repos are included.

## Monitoring

### View Discovery Events

```bash
# Show all discovery-related events
syncctl events list | grep discovery

# Show events for specific discovery run
syncctl trace show discover-1234567890
```

### Check Discovered Repositories

The state database tracks all discovered repositories:

```sql
sqlite3 ~/.config/git-project-sync/state/sync.db

SELECT * FROM discovered_repos WHERE source_id = 'my-github';
SELECT * FROM clone_operations WHERE status = 'success';
```

## Behavior Details

### What Gets Discovered

Auto-discovery includes:
- **Owned repositories**: Repos owned by your user account
- **Organization repositories**: Repos in organizations you're a member of
- **Accessible repositories**: Any repo you have access to via the PAT token

### What Gets Cloned

A repository is cloned if ALL of the following are true:
1. ✅ Auto-clone is enabled in governance policy
2. ✅ Repository matches include patterns (or no patterns specified)
3. ✅ Repository does NOT match exclude patterns
4. ✅ Repository size is under `auto_clone_max_size_mb` limit
5. ✅ Repository is not archived (unless `auto_clone_include_archived: true`)
6. ✅ Repository is not already cloned locally

### Clone Location

Repositories are cloned according to your workspace layout:

```yaml
workspace:
  root: /home/user/repos
  layout: provider-account-repo   # e.g., /home/user/repos/github/myorg/myrepo
```

Supported layouts:
- `flat`: All repos in root directory
- `provider`: Group by provider (github, azuredevops)
- `provider-account`: Group by provider and account/org
- `provider-account-repo`: Full hierarchy (recommended)

## Safety Features

### API Rate Limiting

- Discovery queries are spaced out to avoid rate limits
- Failed API calls are retried with exponential backoff
- Provider-level backoff prevents repeated failures

### Clone Safeguards

- Clones run **sequentially** (one at a time) to prevent disk thrashing
- Failed clones are logged and retried on next discovery cycle
- Permanent failures (e.g., auth errors) are logged but don't block other repos
- Disk space is checked before cloning (future enhancement)

### Non-Blocking Architecture

- Discovery runs in **background goroutine** in daemon mode
- Sync operations continue normally during discovery
- Only one discovery can run at a time (mutex-protected)
- Context cancellation is respected for graceful shutdown

## Troubleshooting

### Discovery Not Running

Check configuration:

```bash
syncctl config get governance.default_policy.auto_clone_enabled
syncctl config get daemon.discovery_interval_seconds
```

Ensure:
- `auto_clone_enabled` is `true`
- `discovery_interval_seconds` is greater than 0 (for periodic discovery)

### No Repositories Found

1. **Check PAT permissions**: Ensure your token has `repo` scope
2. **Check governance patterns**: May be excluding all repos
3. **Check provider connectivity**: Test with `syncctl auth test <source-id>`

### Clone Failures

View failed clones:

```bash
syncctl events list | grep clone_failed
```

Common causes:
- **Authentication failure**: Token expired or insufficient permissions
- **Network errors**: Transient connectivity issues (will retry)
- **Disk space**: Not enough space for repository
- **Size limit**: Repository exceeds `auto_clone_max_size_mb`

## Best Practices

1. **Start Conservative**: Begin with restrictive `include_repo_patterns` and expand gradually
2. **Set Size Limits**: Use `auto_clone_max_size_mb` to prevent cloning huge repos
3. **Monitor Initially**: Watch discovery events during first few cycles
4. **Adjust Interval**: Tune `discovery_interval_seconds` based on your repo creation frequency
5. **Use Dry-Run**: Test governance patterns with `syncctl discover --dry-run` before enabling

## Performance Considerations

### API Call Volume

- Discovery queries **all accessible repos** from each enabled source
- Typical API calls per discovery run:
  - GitHub: 1-10 calls (depending on pagination)
  - Azure DevOps: 1-5 calls per organization
  
- **Recommended**: Set `discovery_interval_seconds` to at least 3600 (1 hour)

### Clone Time

- Cloning is **sequential** (one repo at a time)
- Large repos (>1GB) can take several minutes
- During clone operations, discovery blocks waiting for completion
- **Tip**: Set `auto_clone_max_size_mb` to skip extremely large repos

### Disk Space

- Discovery does NOT check available disk space before cloning
- Monitor disk usage: `df -h $(syncctl config get workspace.root)`
- Use `exclude_repo_patterns` to skip unnecessary large repos

## Security

### Token Requirements

See [PAT Permission Requirements](pat-permission-requirements.md) for required scopes.

**Minimum scopes for discovery**:
- **GitHub**: `repo` (full control of private repositories)
- **Azure DevOps**: `vso.code` (Code - Read)

### Token Storage

- Tokens stored in OS keyring (Linux: libsecret, Windows: Credential Manager)
- Fallback to encrypted file if keyring unavailable
- Never logged or written to state database
- See [Security Model](../security/security-model-and-controls.md) for details

## Related Commands

- `syncctl discover` - Manual discovery trigger
- `syncctl repo clone` - Clone specific repository
- `syncctl events list` - View discovery events
- `syncctl trace show <trace-id>` - View discovery details
- `syncctl auth test <source-id>` - Test API connectivity

## Configuration Reference

See [Configuration Schema](configuration-schema.md) for full details on all auto-discovery settings.
