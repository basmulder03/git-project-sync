# AI Runbook

Use this runbook when assigning work to coding agents.

## 1) Bootstrap Prompt

Provide this context to the agent:

- Read and follow `AGENTS.md`.
- Treat `docs/REQUIREMENTS.md` as product source of truth.
- Match behavior to `docs/ACCEPTANCE_TESTS.md`.
- Keep CLI behavior aligned with `docs/CLI_SPEC.md`.
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

Return:
1) Files changed
2) What was implemented
3) Tests run + results
4) Remaining risks or TODOs
```
