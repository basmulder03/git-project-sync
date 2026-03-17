# CLI-TUI Parity Matrix

This reference defines command-group parity between `syncctl` and `synctui`.

## Why this exists

- Prevents drift between CLI and TUI capabilities.
- Makes parity coverage explicit for operators and maintainers.
- Enables CI guardrails so newly added CLI groups must be represented in TUI pathways.

## Source of truth

- Machine-readable matrix: `docs/reference/cli-tui-parity-matrix.yaml`
- Parity tests: `tests/integration/parity/parity_matrix_test.go`

## Enforcement

Parity tests verify that:

1. Every `syncctl` top-level command group is present in the matrix.
2. Every matrix-mapped `tui_palette` command exists in the TUI command palette catalog.

CI runs `go test ./tests/integration/parity/...` to enforce this contract.

## Current parity coverage

Covered top-level groups include:

- `doctor`, `source`, `repo`, `workspace`, `sync`, `discover`
- `daemon`, `config`, `auth`, `cache`, `stats`, `events`, `trace`, `state`
- `install`, `uninstall`, `service`, `update`, `maintenance`
