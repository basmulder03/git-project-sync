# Test Strategy

## Scope

The test strategy combines fast package-level verification with integration coverage for safety-critical flows.

- `go test ./...` validates all packages.
- `go test ./tests/integration/...` validates cross-package behavior and platform flows.
- Reliability-focused integration tests are run explicitly for soak/failure/rate-limit scenarios in CI.

## Coverage Inventory and Gates

CI enforces minimum package coverage for critical components through `scripts/ci/coverage.sh`.

Current minimum thresholds:

| Package | Minimum Coverage |
| --- | ---: |
| `cmd/syncctl` | 55% |
| `internal/core/daemon` | 75% |
| `internal/core/git` | 65% |
| `internal/core/install` | 60% |
| `internal/core/state` | 65% |
| `internal/core/sync` | 70% |
| `internal/core/update` | 65% |
| `internal/core/workspace` | 75% |

The coverage script:

1. Executes package-specific coverage profiles.
2. Computes package totals.
3. Fails CI if any critical package drops below threshold.
4. Publishes artifacts (`coverage/summary.txt`, `coverage/report.md`, and per-package profiles).

## Threshold Exceptions and Rationale

- `cmd/syncd` and `cmd/synctui` are thin process entrypoints, so coverage value is low relative to effort; behavior is exercised through package tests and integration scenarios.
- `cmd/syncctl` threshold is lower than core packages because command wiring and IO formatting paths have a larger surface and are partially validated through integration/command tests.
- Thresholds are intentionally incremental and should be raised as new tests are added.

## Local Verification

Run the same checks used by CI:

```bash
go test ./...
go test ./tests/integration/...
./scripts/ci/coverage.sh
```
