# Contributing

Thanks for contributing to `git-project-sync`.

## Development Workflow

1. Read `AGENTS.md` and relevant docs in `docs/`.
2. Align implementation with sprint/backlog items in `ai/`.
3. Keep changes cohesive and safety-first.
4. Run tests locally before opening PR.

## Local Validation

```bash
go test ./...
go test ./tests/integration/...
./scripts/ci/coverage.sh
```

## Commit Guidance

- Group one cohesive change per commit.
- Include tests and docs updates with behavior changes.
- Use clear conventional-style messages (e.g. `feat(...)`, `fix(...)`, `docs(...)`, `test(...)`).

## Safety Rules

- Never introduce destructive git behavior in sync automation paths.
- Preserve dirty-worktree skip guarantees.
- Preserve traceability and reason-code logging.

## Docs Contribution

- Update relevant docs whenever behavior changes.
- Docs site builds on docs-only changes via GitHub Actions.
- Keep operator-facing instructions command-accurate.
