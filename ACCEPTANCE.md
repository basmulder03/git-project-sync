# ACCEPTANCE.md

## Manual test

1) Configure root (`config init --root <path>`)
2) Add AzDO target (`target add --provider azure-devops --scope <org> <project>`) or org-wide (`target add --provider azure-devops --scope <org>`)
3) Store PAT in keyring (`token set --provider azure-devops --scope <org> <project> --token <pat>`)
4) Run sync. Interactive: `sync`. Non-interactive: `sync --non-interactive --missing-remote <archive|remove|skip>`. Clones missing repos into <root>/azure-devops/<org>/<project>/<repo>. Re-run sync: if clean fast-forward only; if dirty skip modifying working tree; if default branch diverged skip modifying working tree.
5) Delete a remote repo and run sync. Interactive: prompt to remove / archive / skip. Non-interactive: honors missing-remote policy. Archive moves repo under <root>/_archive/azure-devops/<org>/<project>/<repo>.
6) Daemon run (`daemon --run-once --missing-remote <archive|remove|skip>`): syncs only repos in today’s 7-day bucket.
7) Service install/uninstall:
   - Linux: `service install` creates user systemd unit and enables it; `service uninstall` disables and removes it.
   - macOS: `service install` writes LaunchAgent and loads it; `service uninstall` unloads and removes it.
   - Windows: `service install` creates a Scheduled Task; `service uninstall` removes it.
8) For an existing repo with a missing or mismatched `origin` URL, rerun sync and verify origin is reset to the expected remote URL.
9) Run any command (e.g., `sync`) and verify a new JSONL audit entry is appended under the OS data directory `audit/` folder, including failures when commands error.
10) Run `health --provider <provider> --scope <scope>` and verify a success/failure message plus a new audit entry.
11) Remove the remote default branch and re-run sync: verify it logs and skips without modifying the local branch.
12) Remove a tracked upstream ref and re-run sync: verify it logs orphaned local branches.
13) Run `sync` twice without `--refresh` and confirm it uses cached repo inventory; then run with `--refresh` to force a fresh provider listing.
14) Run `sync --force-refresh-all` and confirm it bypasses repo inventory cache and syncs all configured targets/repos even when selector flags are provided.
15) In TUI Dashboard, press `f` and confirm a forced full refresh run starts (fresh provider listing across all targets).
16) Verify archived/disabled repos are skipped by default; run `sync --include-archived` to include them.
17) Trigger a failing target in daemon mode and confirm a backoff skip is logged on subsequent runs until the backoff window expires.
18) Register a webhook (`webhook register --provider <provider> --scope <scope> --url <url>`) and verify a success/failure message plus a new audit entry.
19) Run `sync --verify` and confirm mismatched branch refs are logged without modifying non-default branches.
20) Run `cache prune` and confirm it removes cache entries for targets no longer in config.
21) Run `token guide --provider <provider> --scope <scope>` and verify URL + scopes are printed.
22) Run `token validate --provider <provider> --scope <scope>` and verify missing scopes are reported or validation is skipped when unsupported.
23) Run `token doctor --provider <provider> --scope <scope>` and verify it reports DBUS/runtime environment and keyring roundtrip status.
24) Open the TUI: verify Config and Token screens show guidance text and validation feedback when submitting empty/invalid values.
25) Run `install` and confirm the daemon service/task is installed; verify delayed start when `--delayed-start` is provided.
26) Run `install --tui` and confirm the installer screen runs the same flow.
27) Run `install --path add` and confirm PATH registration message is shown.
28) Run `mirror-cli` with no args: if not installed, installer opens; if installed, help is shown.
29) Re-run `install` with a newer binary and confirm it replaces the existing install in the OS default location and restarts the service using that path.
30) Run `install --status` and confirm it prints install path, service/task state, and PATH status.
31) Run `sync --status` and confirm live progress output and final summary.
32) In the TUI dashboard, press `s` to open Sync Status and confirm current action/repo/counts are shown.
33) Run `sync` and confirm per-repo audit entries are created (e.g., `sync.repo` or `daemon.sync.repo`).
34) Start the daemon and confirm an update check is logged on startup, then daily.
35) Run the CLI before any daemon check and confirm it performs a one-time update check; subsequent CLI runs should skip if daemon has checked.
36) Simulate no network and confirm update checks are audited as skipped and do not fail the daemon.
37) Trigger an update with insufficient permissions and confirm the CLI prompts to re-run with elevated permissions.
38) Set an invalid PAT and confirm `token set` rejects it and the token is not stored.
39) Run the daemon and confirm daily PAT validity checks are audited; expired tokens print a warning.
40) Run the CLI before any daemon token check and confirm it performs a one-time PAT validity check; subsequent CLI runs should skip if the daemon has checked.
41) Run `sync --target-id <id> --provider <other> --scope <other>` and confirm target-id wins with an explicit warning.
42) Run `health --target-id <id> --provider <other> --scope <other>` and confirm target-id wins with an explicit warning.
43) In TUI, navigate Main -> Targets -> Add Target, press `Esc`, and confirm it returns to Targets (not Main).
44) In TUI, navigate Main -> Tokens -> Set/Validate, press `Esc`, and confirm it returns to Tokens.
45) In long-content TUI screens (Dashboard, Sync Status, Install Status, Audit Log, Token List), verify `PgUp/PgDn/Home/End` scrolls content when overflowing.
