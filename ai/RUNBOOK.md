# AI Runbook

Use this runbook when assigning work to coding agents.

## 1) Bootstrap Prompt

Provide this context to the agent:

- Read and follow `AGENTS.md`.
- Treat `docs/project/product-requirements.md` as product source of truth.
- Match behavior to `docs/engineering/acceptance-test-matrix.md`.
- Keep CLI behavior aligned with `docs/reference/cli-command-specification.md`.
- Treat logging and traceability as mandatory from first implementation tasks.
- Preserve multi-source and multi-account compatibility in all designs.
- Do not violate safety rules under any condition.

## 2) Work Assignment Pattern

For each task:

1. Reference epic/task IDs from `ai/backlog.yaml`.
2. Ask for a minimal implementation plan.
3. Ask for code changes + tests only for assigned scope.
4. Require test output and explicit pass/fail summary.
5. Require documentation updates if behavior changed.

## 2.1) Commit Discipline (Required)

Agents must create commits at logical checkpoints, not only at the end.

- Commit after completing a cohesive unit of work (feature slice, test slice, refactor slice).
- Each commit must keep the repository in a buildable/testable state for that slice.
- Commit message should explain intent and reason, not only file changes.
- Never mix unrelated work in one commit.
- Run the smallest relevant tests before each commit and include results in handoff.

Recommended commit cadence for implementation tasks:

1. Core behavior commit (implementation only)
2. Test coverage commit (unit/integration tests)
3. Docs/spec alignment commit (if required)

If a task is very small, combine into one commit.

## 3) Definition of Done (Per Task)

- Feature behavior matches acceptance criteria.
- Unit/integration tests added or updated.
- Logs include skip/action reason for safety decisions.
- No destructive git command in automation path.
- No secrets in logs.

## 4) Recommended Task Sequence

1. E1 (core safety/sync)
2. E2 (providers/auth)
3. E3 (daemon)
4. E4 (CLI)
5. E5 (TUI)
6. E6 (install/service)
7. E7 (self-update)
8. E8 (hardening/docs)

## 5) Agent Prompt Template

```text
Implement task <ID> from ai/backlog.yaml.

Constraints:
- Follow AGENTS.md safety rules exactly.
- Keep changes scoped to this task.
- Add/adjust tests proving acceptance behavior.
- Update docs if command/behavior changes.
- Create logical commits during the task; do not wait until all epics are done.

Return:
1) Files changed
2) What was implemented
3) Tests run + results
4) Commits created (hash + message + rationale)
5) Remaining risks or TODOs
```
