# Skill: Daemon Traceability

## Goal

Run reliable background sync with full action traceability.

## Checklist

- Use per-repo lock to prevent concurrent mutation.
- Add jitter, timeout budgets, and retry/backoff.
- Emit run-level trace ID for every scheduler cycle.
- Link each repo job event to the same trace ID.
- Persist structured events for later query.
- Include machine-readable reason codes for skip/failure paths.

## Must-Have Tests

- Concurrent sync attempts on same repo are serialized.
- Trace IDs are present in events and queryable.
- Retry policy behaves correctly for transient failures.

## Commit Pattern

1. Scheduler + locking.
2. Trace/event pipeline.
3. Failure/retry behavior tests.
