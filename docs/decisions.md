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

## OAuth device flow (limited)

**Decision:** Support OAuth device flow for GitHub and Azure DevOps; keep PATs as fallback.

**Rationale:** Device flow improves UX but availability varies; PATs remain universal.

## No PATH modification in installer

**Decision:** The project does not modify PATH automatically.

**Rationale:** Avoids OS policy issues and reduces side effects during install.
