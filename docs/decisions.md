# Git Project Sync — Architecture Decisions

This document captures key architectural decisions and the rationale behind them.

## Rust + workspace layout

**Decision:** Use a Rust workspace with separate crates for core logic, providers, CLI, and optional services.

**Rationale:** Keeps provider-specific concerns isolated and makes the core engine reusable and testable.

## Provider adapter pattern

**Decision:** Providers implement a trait rather than embedding API calls into the core.

**Rationale:** Enforces clean boundaries; adding a new provider does not require core changes.

## Safe git sync (fast-forward only)

**Decision:** Never overwrite dirty working trees or force-reset diverged branches.

**Rationale:** Safety first; prevents data loss and unexpected history rewrites.

## Audit logs as JSONL

**Decision:** Store audit logs as JSONL with file rotation.

**Rationale:** Append-only, easy to parse, works with most log tooling.

## Token storage via OS keychain

**Decision:** Use `keyring` to store tokens.

**Rationale:** Avoids plaintext credentials on disk; uses OS security primitives.

## Staggered scheduler (7‑day buckets)

**Decision:** Hash repos into day buckets; daemon syncs only one bucket per day.

**Rationale:** Spreads load across the week and reduces API pressure.

## Cache for repo inventory

**Decision:** Cache repo lists per target with TTL.

**Rationale:** Reduces API calls and speeds up frequent syncs.

## Credential-free repo inventory

**Decision:** Keep provider credentials out of `RemoteRepo` inventory records and resolve auth via `auth_for_target` during sync execution.

**Rationale:** Keeps inventory data provider-agnostic and non-sensitive, and simplifies provider/cache boundaries.

## PATH modification is opt-in

**Decision:** The installer only modifies PATH when explicitly requested.

**Rationale:** Avoids OS policy issues and reduces side effects during install while still offering convenience.

## Default install location + manifest

**Decision:** The installer copies the binary into the OS default per-user install location and writes an `install.json` manifest.

**Rationale:** Uses predictable OS-appropriate locations and enables reliable update/replace behavior.

## Install marker file (legacy)

**Decision:** Keep a marker file in OS app data for backward-compatible install detection.

**Rationale:** Ensures older installs remain detectable while migrating to the manifest.
