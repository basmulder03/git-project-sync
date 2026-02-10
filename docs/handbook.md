# Git Project Sync — Handbook

This handbook is the operator-focused guide to installing, configuring, and running Git Project Sync.

## Purpose

Git Project Sync mirrors repositories from multiple Git providers into a local root folder. It keeps local repos up to date safely, without overwriting dirty working trees and without force-resets. Providers are implemented through adapters so the core engine stays provider-agnostic.

## Installation

### Quick Install (Recommended)

The fastest way to install Git Project Sync is using our installation scripts:

#### Linux/macOS

```bash
curl -fsSL https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/install.sh | bash
```

This will:
- Detect your OS and architecture
- Download the latest release binary
- Install to `~/.local/bin/mirror-cli`
- Provide instructions for adding to PATH if needed

You can customize the installation directory:

```bash
INSTALL_DIR=/usr/local/bin curl -fsSL https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/install.sh | bash
```

#### Windows

Open PowerShell and run:

```powershell
iwr -useb https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/install.ps1 | iex
```

To automatically add to PATH:

```powershell
iwr -useb https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/install.ps1 | iex -AddToPath
```

The default installation directory is `%LOCALAPPDATA%\Programs\mirror-cli`.

### Download Pre-built Binaries

Download pre-built binaries from [GitHub Releases](https://github.com/basmulder03/git-project-sync/releases):

- `mirror-cli-linux-x86_64` - Linux x86_64
- `mirror-cli-macos-x86_64` - macOS x86_64  
- `mirror-cli-windows-x86_64.exe` - Windows x86_64

After downloading:

**Linux/macOS:**
```bash
chmod +x mirror-cli-*
sudo mv mirror-cli-* /usr/local/bin/mirror-cli
```

**Windows:**
Move the `.exe` file to a directory in your PATH.

### Build from Source

Build the CLI:

```bash
cargo build --release
```

Run the binary:

```bash
./target/release/mirror-cli --help
```

Install to system:

```bash
# Linux/macOS
sudo cp target/release/mirror-cli /usr/local/bin/

# Or to user directory
cp target/release/mirror-cli ~/.local/bin/
```

## Updating

### Automatic Update via Scripts

**Linux/macOS:**
```bash
curl -fsSL https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/update.sh | bash
```

**Windows (PowerShell):**
```powershell
iwr -useb https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/update.ps1 | iex
```

These scripts will:
- Check your current version
- Download the latest release if newer
- Update the binary in place
- Preserve your installation location

### Built-in Update Command

Git Project Sync includes a built-in update mechanism:

```bash
# Check for available updates
mirror-cli update --check-only

# Check and apply updates during sync
mirror-cli sync --check-updates

# Apply updates immediately
mirror-cli update --apply
```

Override the update source repository:

```bash
GIT_PROJECT_SYNC_UPDATE_REPO=owner/repo mirror-cli update --check-only
```

## Verifying Installation

After installation, verify that `mirror-cli` is working:

```bash
# Check if command is available
mirror-cli --version

# View help
mirror-cli --help
```

If the command is not found, you may need to add the installation directory to your PATH:

**Linux/macOS:**
Add to your shell profile (`~/.bashrc`, `~/.zshrc`, etc.):

```bash
export PATH="$HOME/.local/bin:$PATH"
```

Then reload:
```bash
source ~/.bashrc  # or ~/.zshrc
```

**Windows:**
1. Open System Properties → Environment Variables
2. Edit the user `Path` variable
3. Add the installation directory (e.g., `%LOCALAPPDATA%\Programs\mirror-cli`)
4. Restart your terminal

### PATH behavior

PATH updates are **opt-in** during installation. The installation scripts will notify you if the binary is not in your PATH and provide instructions.

For the built-in `mirror-cli install` command:
- Windows: adds the install folder to the user PATH environment variable
- macOS/Linux: creates a symlink to a PATH location, for example `ln -s <install-dir>/mirror-cli /usr/local/bin/mirror-cli`

## Releases

Releases are created manually via the GitHub Actions **Release** workflow. Choose the bump type (major, minor, patch) when dispatching; the workflow updates versions, creates the tag, and publishes binaries.

Quick trigger with the GitHub CLI:

```bash
gh workflow run Release -f bump=patch
```

**Release Checklist**
1. Run `cargo fmt --check`, `cargo clippy --all-targets --all-features -- -D warnings`, and `cargo test --all`.
2. Dispatch the **Release** workflow with the desired bump type.

## Configuration

Initialize config with a mirror root:

```bash
mirror-cli config init --root /path/to/mirrors
```

Targets are defined by provider + scope. Scopes differ by provider:

- Azure DevOps: `<org>` or `<org>/<project>`
- GitHub: `<org-or-user>`
- GitLab: `<group>/<subgroup>/...`

Add targets:

```bash
mirror-cli target add --provider azure-devops --scope org project
mirror-cli target add --provider github --scope org-or-user
mirror-cli target add --provider gitlab --scope group subgroup
```

List/remove targets:

```bash
mirror-cli target list
mirror-cli target remove --target-id <id>
```

## Token Management

Tokens are stored in the OS keychain via `keyring`.

### PATs

Store a PAT:

```bash
mirror-cli token set --provider azure-devops --scope org project --token <pat>
```

Tokens are validated on set; invalid or expired PATs are rejected.

GitHub note: use a Fine-grained personal access token (classic PATs are deprecated).
The CLI `token guide` command prints the current GitHub token URL and required permissions.

Get guidance:

```bash
mirror-cli token guide --provider github --scope org-or-user
```

Validate scopes (when supported):

```bash
mirror-cli token validate --provider github --scope org-or-user
```

Diagnose keyring/session issues:

```bash
mirror-cli token doctor --provider github --scope org-or-user
```

## Sync Behavior

For each configured target, the engine:

- Lists remote repos.
- Clones missing repos.
- For existing repos:
  - If working tree clean: fetch + fast-forward the default branch only.
  - If dirty or diverged: skip changes and log.
  - If origin URL missing or mismatched: update origin to expected URL before fetch.
  - If default branch renamed: create missing local branch and log.
  - If default branch missing on remote: skip and log.
  - Detects orphaned local branches (upstream missing) and logs.

Remote repo deleted:

- Interactive: prompt for archive/remove/skip.
- Non-interactive: honors `--missing-remote` policy.

## Cache and Scheduling

- Repo lists are cached with a TTL. Use `--refresh` to bypass.
- Daemon runs only the current day bucket (7-day staggering).
- Backoff is applied per target on repeated failures.
- Target last-success timestamps are tracked for monitoring.

## Running a Sync

```bash
mirror-cli sync
```

Non-interactive policy:

```bash
mirror-cli sync --non-interactive --missing-remote <archive|remove|skip>
```

Verify refs without modifying non-default branches:

```bash
mirror-cli sync --verify
```

## Daemon and Service Install

Daemon loop:

```bash
mirror-cli daemon --missing-remote skip
```

Run one cycle:

```bash
mirror-cli daemon --run-once --missing-remote skip
```

Service install:

```bash
mirror-cli service install
mirror-cli service uninstall
```

OS behavior:

- Linux: installs a systemd user service.
- macOS: installs a LaunchAgent.
- Windows: installs a Windows service.

## Installer Flow

The installer sets up the daemon and optionally registers PATH.

```bash
mirror-cli install
```

Interactive TUI installer:

```bash
mirror-cli install --tui
```

Options:

- `--delayed-start <seconds>`: delayed startup on boot (OS-native).
- `--path <add|skip>`: opt-in PATH registration.

Default install locations (per-user):

- Windows: `%LOCALAPPDATA%\Programs\git-project-sync\mirror-cli.exe`
- macOS: `~/Library/Application Support/git-project-sync/bin/mirror-cli`
- Linux: `~/.local/share/git-project-sync/bin/mirror-cli`

### Default launch behavior

When you run the executable without arguments:

- If **not installed**, it starts the TUI installer.
- If **installed**, it shows the CLI help screen.

Installation is tracked via an install marker file stored under the OS data directory.
The installer also writes an `install.json` manifest to the same location and uses it to detect existing installs.
Only one installer can run at a time (guarded by an install lock file under the same directory).
Re-running the installer replaces the existing install in place and updates the service to point at the new binary.
The installer lock is re-entrant within the same process, so updates can run while the UI holds the lock.

## Updates

Check for updates:

```bash
mirror-cli update --check-only
```

Check updates alongside any command:

```bash
mirror-cli sync --check-updates
```

Apply the latest release:

```bash
mirror-cli update --apply
```

Updates require an existing install. If none is found, run `mirror-cli install` first.

Override the GitHub release repo:

```bash
GIT_PROJECT_SYNC_UPDATE_REPO=owner/repo mirror-cli update --check-only
```

In the TUI, use the "Update" action from the main menu, press `u` in the installer view, or press `u` on the dashboard.

Auto-update behavior:

- The daemon checks for updates on startup and then daily.
- CLI runs check for updates only if the daemon has not performed a check yet.
- Network failures are logged and do not fail the daemon.
- If updates require elevated permissions, the CLI prompts to re-run and triggers the OS admin-privileges prompt (UAC/sudo/etc.).

PAT validation behavior:

- Tokens are validated on set; invalid or expired PATs are rejected.
- The daemon checks PAT validity daily.
- CLI checks PAT validity only if the daemon has not performed a check yet.

## Dashboard

Dashboard TUI:

```bash
mirror-cli tui --dashboard
```

The dashboard shows core stats and can toggle per-target status with `t`.

## TUI

Launch the terminal UI:

```bash
mirror-cli tui
```

The TUI includes config, targets, tokens, audit logs, and dashboard views.

## Logs and Audit

Audit logs are JSONL files stored under the OS data directory (`audit/`). Entries include command, status, and context. Rotate when size limit is reached.

## Troubleshooting

Common issues:

- Dirty working tree: repo is skipped by design.
- Diverged default branch: repo is skipped by design.
- Missing origin: origin URL is restored automatically.
- PATH not updated: rerun `mirror-cli install --path add` or update PATH manually.

## Safety Guarantees

- No force resets.
- No overwrite of dirty working trees.
- Fast-forward only for default branch.
- Diverged or missing default branch is logged and skipped.
