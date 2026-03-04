# git-project-sync

Cross-platform Git repository synchronizer for local repositories.

This project is intended to be built with AI coding agents. Start with `AGENTS.md`, then follow the docs in `docs/` and structured tasks in `ai/`.

## What It Will Do

- Keep local repositories updated with their remote default branch (`main`/`master`).
- Sync safely in a background daemon.
- Offer full control through a CLI.
- Provide a lightweight TUI dashboard.
- Support GitHub and Azure DevOps PAT authentication.
- Support multiple provider sources/accounts at the same time.
- Avoid all destructive behavior and skip dirty repositories.
- Enforce logging and traceability from the first implementation phase.

## Build Order

1. Core sync and git safety engine
2. Provider integrations
3. Daemon
4. CLI
5. TUI
6. Installer/service registration
7. Self-update
8. Hardening, tests, docs
