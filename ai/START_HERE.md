# Start Here (AI Agents)

Use this file as your entry point before implementing anything.

## Read Order

1. `AGENTS.md`
2. `ai/master-plan.yaml`
3. `docs/project/product-requirements.md`
4. `docs/engineering/system-architecture.md`
5. `docs/engineering/acceptance-test-matrix.md`
6. `docs/reference/cli-command-specification.md`
7. `docs/reference/configuration-schema.md`
8. `docs/reference/pat-permission-requirements.md`
9. `ai/backlog.yaml`
10. Current sprint file (`ai/sprint-01.yaml` ... `ai/sprint-10.yaml`)
11. `ai/RUNBOOK.md`
12. `ai/skills/skills.yaml` and relevant skill file(s)

## Execution Rules

- Follow safety rules in `AGENTS.md` without exception.
- Treat logging and traceability as mandatory from the first implemented task.
- Keep sensitive data out of config and logs.
- Support multi-source/multi-account GitHub and Azure DevOps behavior.
- Use managed workspace layout conventions.

## Commit Discipline (Required)

- Create commits at logical checkpoints.
- Keep each commit cohesive and test-backed.
- Do not bundle unrelated changes.
- Include commit hash/message/rationale in your handoff.

## Suggested Task Prompt

```text
Implement task <ID> from ai/backlog.yaml and the active sprint file.

Requirements:
- Follow AGENTS.md safety rules exactly.
- Keep scope limited to this task.
- Add/update tests for acceptance behavior.
- Update docs/spec where behavior changes.
- Create logical commits during implementation.

Return:
1) Files changed
2) What was implemented
3) Tests run + results
4) Commits created (hash + message + rationale)
5) Remaining risks/TODOs
```
