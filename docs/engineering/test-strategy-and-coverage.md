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

## Test Determinism Rules

All tests in this repository must be deterministic across repeated runs. Violations cause CI flakiness and erode confidence.

**Rules:**

1. **No fixed-delay timing gates.** Do not use `time.Sleep` to wait for an observable side effect. Use `waitForCondition` / `assertEventually` from `tests/integration/helpers_test.go` instead.
2. **No random seeds without explicit seeding.** If a test depends on random ordering, seed `math/rand` with a fixed value and document why.
3. **Isolated state per test.** Every test that touches the database or filesystem must use `t.TempDir()` or an in-memory fixture, never a shared path.
4. **Parallel-safe mocks.** All shared state inside a parallel test must be protected with a mutex or atomic. Read the mock state only after `RunCycle` or the equivalent synchronous call returns.
5. **Deterministic iteration order.** When asserting counts over a `map`, sort the keys before iterating or use `t.Fatalf` with the full map value so failures are reproducible.

**Acceptable use of `time.Sleep`:**

- Simulating work duration in a mock worker (e.g., `time.Sleep(4ms)` in a contention test). This must not gate any assertion; only the result of the cycle completion is asserted.

## Flake Triage Playbook

### Step 1 — Reproduce locally

```bash
go test ./tests/integration/... -run <TestName> -count=10 -v 2>&1 | grep -E 'FAIL|PASS|---'
```

If the failure reproduces fewer than 3 times in 10 runs, classify as **intermittent**; fewer than 1 in 10 runs, classify as **rare**.

### Step 2 — Root-cause categories

| Symptom | Likely cause | Fix |
| --- | --- | --- |
| Assertion fails on count/order | Map iteration non-determinism | Sort keys; use sorted slice |
| Race detector (`-race`) report | Shared mutable state without lock | Add mutex / atomic |
| SQLITE_BUSY or lock error | Shared DB path across parallel tests | Use `t.TempDir()` for each test |
| Timestamp assertion failure | Clock skew or `time.Now()` race | Use event count, not wall time |
| Port already in use | Port collision in parallel tests | Use `:0` (OS-assigned port) |
| Unexpected skip | Missing `git` in PATH | Add `exec.LookPath("git")` guard |

### Step 3 — Quarantine process

If a test cannot be fixed immediately:

1. Add the comment `// [QUARANTINE] reason: <short description> tracked: <issue URL>` directly above the test function.
2. Add the test name to the `-run` exclusion regex in the **Integration flake-rerun** CI step.
3. Open a tracking issue with label `flaky-test` and link it in the comment.
4. The quarantine must not last more than one sprint. Unresolved quarantines block the sprint's Definition of Done.

### Step 4 — Rerun policy

CI automatically runs `go test ./tests/integration/... -count=3`. A test is considered stable only if it passes all three runs. A test that passes 2 out of 3 must be triaged before merging to `main`.

### Step 5 — Graduate from quarantine

1. Run the test 20 consecutive times locally without failure.
2. Remove the `// [QUARANTINE]` comment and the exclusion from the CI rerun step.
3. Close the tracking issue.

