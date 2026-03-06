# git-project-sync

[![CI](https://github.com/basmulder03/git-project-sync/actions/workflows/ci.yml/badge.svg)](https://github.com/basmulder03/git-project-sync/actions/workflows/ci.yml)
[![Docs](https://github.com/basmulder03/git-project-sync/actions/workflows/docs.yml/badge.svg)](https://github.com/basmulder03/git-project-sync/actions/workflows/docs.yml)
[![Release](https://github.com/basmulder03/git-project-sync/actions/workflows/release.yml/badge.svg)](https://github.com/basmulder03/git-project-sync/actions/workflows/release.yml)
[![Go Version](https://img.shields.io/badge/go-1.26-00ADD8?logo=go)](https://go.dev/)
[![Docs Site](https://img.shields.io/badge/docs-gh--pages-2ea44f)](https://basmulder03.github.io/git-project-sync/)

Cross-platform Git repository synchronizer focused on safe automation, operational traceability, and release-ready workflows.

## At a glance

- Safe sync automation for local repositories
- Linux + Windows support
- Multi-source account support (GitHub + Azure DevOps)
- Full CLI + operations TUI + setup TUI
- Traceable reason codes and incident-ready diagnostics

## Architecture snapshot

```text
+-----------+       +-------------------+       +-------------------+
|  syncctl  +------->      syncd        +-------> local git repos   |
|   CLI     |       | daemon scheduler  |       | + provider remotes |
+-----+-----+       +---------+---------+       +-------------------+
      |                         |
      |                         v
      |               +-------------------+
      +---------------> SQLite state DB   |
                      | events / runs /   |
                      | repo status       |
                      +---------+---------+
                                |
                                v
                      +-------------------+
                      |      synctui      |
                      | runtime dashboard |
                      +-------------------+
```

## Why This Project

`git-project-sync` keeps local repositories aligned with their remote default branch (`main`/`master`) while enforcing strict safety guardrails:

- never mutate dirty repositories,
- never use destructive git automation paths,
- always emit traceable reason codes for skipped/failed actions.

It includes runtime and setup interfaces:

- `syncctl` (CLI control plane),
- `synctui` (operations dashboard),
- `syncd` (background daemon).

## Getting started in 2 minutes

1. Install: [Installation and Service Registration](https://basmulder03.github.io/git-project-sync/getting-started/installation-and-service-registration)
2. Onboard: [First-Run Onboarding](https://basmulder03.github.io/git-project-sync/getting-started/first-run-onboarding)
3. Local dev from source: [Local Development Flow](https://basmulder03.github.io/git-project-sync/getting-started/local-development-flow)
4. Validate health with `syncctl doctor`.

## Quick Start

Install and onboard:

1. Follow [Installation and Service Registration](https://basmulder03.github.io/git-project-sync/getting-started/installation-and-service-registration).
2. Follow [First-Run Onboarding](https://basmulder03.github.io/git-project-sync/getting-started/first-run-onboarding).

Minimal first-run command flow:

```bash
syncctl source add github gh-personal --account <account>
syncctl auth login gh-personal --token <pat>
syncctl repo add /path/to/repo --source-id gh-personal
syncctl sync all --dry-run
syncctl sync all
syncctl doctor
```

Open dashboards:

```bash
synctui
```

## Release Process

Use the `Release` GitHub Actions workflow (`workflow_dispatch`) to do a one-click release:

- provide `version` and `target_ref`,
- workflow validates the version, creates/pushes the tag, builds artifacts, generates checksums/SBOM/manifest, and publishes the GitHub Release.

Details: [Release Process and Automation](https://basmulder03.github.io/git-project-sync/release/release-process-and-automation).

## Reliability focus

- SLOs and error budgets: [Reliability SLOs and Error Budgets](https://basmulder03.github.io/git-project-sync/operations/reliability-slos-and-error-budgets)
- Acceptance closure and verification mapping: [Acceptance Test Matrix](https://basmulder03.github.io/git-project-sync/engineering/acceptance-test-matrix)
- RC and rollback gates: [Release Candidate Checklist](https://basmulder03.github.io/git-project-sync/release/release-candidate-checklist)

## Built with AI Agent Workflow

This repository is structured for autonomous implementation with coding agents.

- Agent entrypoint: `AGENTS.md`
- Planning and sprint execution: `ai/`
