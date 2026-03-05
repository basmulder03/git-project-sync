# PAT Permissions

This document defines minimum token scopes for supported providers.

## GitHub

### Fine-grained PAT (recommended)

- Repository access: selected repositories
- Permission required: `Contents: Read`

Use only read permission for v1 because sync behavior is fetch/pull based and does not push.

### Classic PAT (fallback)

- Private repositories: `repo`
- Public repositories only: `public_repo`

## Azure DevOps

- Scope required: `Code (Read)`

Use read-only scope for v1.

## Storage and Handling Rules

- Never store PAT in plaintext config files.
- Use OS keychain/keyring if available.
- If OS keyring is unavailable, use encrypted fallback storage (AES-GCM) and require a runtime key via environment variable.
- Support multiple active PATs at once (one per configured source/account).
- Store PAT entries keyed by source ID so personal and corporate accounts can coexist.
- Redact token values in logs and UI.
- Validate token immediately on login command.
- Allow token rotation without deleting repo config.

### Secure Fallback Details

- Fallback token file contains encrypted payload only; token values are never persisted in plaintext.
- Required environment variable: `GIT_PROJECT_SYNC_FALLBACK_KEY` (or configured equivalent).
- Fallback file permissions should be owner-only (`0600` on Unix-like systems).
- If no keyring and no fallback key is available, login/token-write actions must fail with an actionable error.

## Future Scope Changes

If write features (push/rebase/branch create) are introduced in later versions, revisit required permissions and update this file before release.
