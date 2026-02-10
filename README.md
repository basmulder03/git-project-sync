# Git Project Sync

[![CI](https://github.com/basmulder03/git-project-sync/actions/workflows/ci.yml/badge.svg)](https://github.com/basmulder03/git-project-sync/actions/workflows/ci.yml)
[![Release](https://github.com/basmulder03/git-project-sync/actions/workflows/release.yml/badge.svg)](https://github.com/basmulder03/git-project-sync/actions/workflows/release.yml)
[![Latest Release](https://img.shields.io/github/v/release/basmulder03/git-project-sync)](https://github.com/basmulder03/git-project-sync/releases)
[![Rust](https://img.shields.io/badge/Rust-2024-orange?logo=rust)](https://www.rust-lang.org)
[![Platforms](https://img.shields.io/badge/Platforms-Windows%20%7C%20macOS%20%7C%20Linux-blue)](https://github.com/basmulder03/git-project-sync/releases)

Mirror repositories from multiple Git providers into a local root folder with safe, fast-forward-only updates.

## Why this tool

- Provider-agnostic core with Azure DevOps, GitHub, and GitLab adapters
- Safe sync: never overwrites dirty trees, fast-forward only, logs diverged branches
- Missing remote handling with interactive prompt or policy (`archive|remove|skip`)
- 7-day staggered daemon sync bucket strategy
- Tokens in OS keychain, config/cache in OS app data directories
- Localized CLI/TUI text with selectable language

## Install

### Quick Install (Recommended)

**Linux/macOS:**
```bash
curl -fsSL https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/install.sh | bash
```

**Windows (PowerShell):**
```powershell
iwr -useb https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/install.ps1 | iex
```

To add to PATH on Windows, download and run with parameter:
```powershell
iwr -Uri https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/install.ps1 -OutFile install.ps1
.\install.ps1 -AddToPath
```

### Download Pre-built Binaries

Download the latest release from [GitHub Releases](https://github.com/basmulder03/git-project-sync/releases):

- `mirror-cli-linux-x86_64` - Linux x86_64
- `mirror-cli-macos-x86_64` - macOS x86_64
- `mirror-cli-windows-x86_64.exe` - Windows x86_64

### Build from Source

Build with Cargo:

```bash
cargo build --release
```

Binary path:

```bash
target/release/mirror-cli
```

## Update

### Quick Update

**Linux/macOS:**
```bash
curl -fsSL https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/update.sh | bash
```

**Windows (PowerShell):**
```powershell
iwr -useb https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/update.ps1 | iex
```

### Using Built-in Update Command

```bash
# Check for updates
mirror-cli update --check-only

# Apply updates
mirror-cli update --apply
```

## Quick Start

1. Initialize config root.

```bash
mirror-cli config init --root /path/to/mirrors
```

2. Add a target.

```bash
# Azure DevOps project scope
mirror-cli target add --provider azure-devops --scope org project

# Azure DevOps org scope
mirror-cli target add --provider azure-devops --scope org

# GitHub org/user
mirror-cli target add --provider github --scope org-or-user

# GitLab group/subgroup
mirror-cli target add --provider gitlab --scope group subgroup
```

3. Store a token.

```bash
mirror-cli token set --provider azure-devops --scope org project --token <pat>
mirror-cli token set --provider github --scope org-or-user --token <token>
mirror-cli token set --provider gitlab --scope group --token <token>
```

4. Run sync.

```bash
mirror-cli sync
```

## Language and Localization

Supported locales:

- `en-001` (English International, default)
- `en-US` (English American)
- `en-GB` (English British)
- `nl` (Dutch)
- `af` (Afrikaans)

Set persisted language:

```bash
mirror-cli config language set --lang en-001
```

One-off override:

```bash
mirror-cli --lang nl sync
```

Precedence:

`--lang` > `MIRROR_LANG` > `config.language` > `en-001`

TUI language switch:

- Open `Language` from the main menu.
- Selection is applied immediately and persisted.

## Common Commands

### Sync

```bash
# Standard sync
mirror-cli sync

# Show live progress
mirror-cli sync --status

# Force full refresh for all targets/repos
mirror-cli sync --force-refresh-all

# Non-interactive missing remote policy
mirror-cli sync --non-interactive --missing-remote <archive|remove|skip>
```

### Health and Cache

```bash
mirror-cli health --provider github --scope org-or-user
mirror-cli cache prune
mirror-cli cache overview
```

### Token workflows

```bash
mirror-cli token guide --provider github --scope org-or-user
mirror-cli token validate --provider github --scope org-or-user
mirror-cli token doctor --provider github --scope org-or-user
```

### Update workflows

```bash
mirror-cli update --check-only
mirror-cli update --apply
mirror-cli sync --check-updates
```

Override update source repo:

```bash
GIT_PROJECT_SYNC_UPDATE_REPO=owner/repo mirror-cli update --check-only
```

## Daemon and Service

Run daemon continuously:

```bash
mirror-cli daemon --missing-remote skip
```

Run one cycle:

```bash
mirror-cli daemon --run-once --missing-remote skip
```

Install/uninstall OS background integration:

```bash
mirror-cli service install
mirror-cli service uninstall
```

## TUI Shortcuts

- Dashboard: `s` (sync status), `f` (force refresh all)
- Setup: `s` (setup status), `u` (check updates)
- Navigation: `Esc` back, `PgUp/PgDn/Home/End` scroll

## Mirror Layout

```text
<root>/
  azure-devops/<org>/<project>/<repo>/
  github/<org>/<repo>/
  gitlab/<group>/<subgroup>/.../<repo>/
  _archive/...
```

## Scope Shapes

- Azure DevOps: `<org>` or `<org>/<project>`
- GitHub: `<org-or-user>`
- GitLab: `<group>/<subgroup>/...`

## Config, Cache, Token Storage

- Config: `config.json` in OS config dir
- Cache: `cache.json` in OS cache dir
- Lock: `mirror.lock` in OS runtime dir (or cache fallback)
- Tokens: OS keychain via `keyring`

## Install Flow

```bash
mirror-cli install
mirror-cli install --tui
mirror-cli install --status
mirror-cli install --path add
```

Optional flags:

- `--delayed-start <seconds>`
- `--path <add|skip>`
- `--status`

Default no-arg behavior:

- Not installed: opens installer
- Installed: shows CLI help

## Troubleshooting

- Dirty working tree: sync skips and logs
- Diverged default branch: sync skips and logs
- Missing/mismatched `origin`: sync rewrites origin URL before fetch
- Azure DevOps 401/403/404: CLI prints scope/token guidance

## Release

Manual release via GitHub Actions workflow `Release` (`major|minor|patch`).

```bash
gh workflow run Release -f bump=patch
```

Release quality gate:

1. `cargo fmt --check`
2. `cargo clippy --all-targets --all-features -- -D warnings`
3. `cargo test --all`

## Documentation

- `docs/handbook.md` - operator guide
- `docs/architecture.md` - system architecture
- `docs/decisions.md` - architecture decisions
- `SPEC.md` - behavioral specification
- `ACCEPTANCE.md` - manual acceptance scenarios
