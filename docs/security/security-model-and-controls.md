# Security Model

This document summarizes security controls for `git-project-sync`.

## Credential Handling

- PATs are validated per source (`syncctl auth test`).
- Sensitive credentials are stored in OS credential storage (or secure fallback).
- Tokens are never persisted in plain config/state files.

See PAT scope requirements in `docs/PAT_PERMISSIONS.md`.

## Data Classification

- Sensitive:
  - PAT values
  - Credential-store references that imply secret material
- Non-sensitive:
  - Config settings
  - Repository state metadata
  - Event and run traces

## Logging and Redaction

- Secret redaction is enabled by default in logging config.
- Reason codes are used for diagnostics instead of sensitive payload detail.

## Git Safety Controls

- No destructive git operations in automation paths.
- No sync mutations on dirty working trees.
- No auto-stash.
- Branch cleanup requires merge safety and unique-commit guard.

## Release Security Gates

- CI/release include:
  - vulnerability scan (`govulncheck`)
  - secret scan (`gitleaks`)
  - checksums for artifacts
  - SBOM generation (SPDX + CycloneDX)

## Operator Recommendations

- Rotate PATs periodically and after any suspected leak.
- Use least privilege PAT scopes.
- Audit event history for repeated auth failures and abnormal update activity.
