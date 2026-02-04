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
14) Verify archived/disabled repos are skipped by default; run `sync --include-archived` to include them.
15) Trigger a failing target in daemon mode and confirm a backoff skip is logged on subsequent runs until the backoff window expires.
16) Register a webhook (`webhook register --provider <provider> --scope <scope> --url <url>`) and verify a success/failure message plus a new audit entry.
17) Run `sync --verify` and confirm mismatched branch refs are logged without modifying non-default branches.
18) Run `cache prune` and confirm it removes cache entries for targets no longer in config.
19) Run `token guide --provider <provider> --scope <scope>` and verify URL + scopes are printed.
20) Run `token validate --provider <provider> --scope <scope>` and verify missing scopes are reported or validation is skipped when unsupported.
21) Run `oauth device --provider github --scope <scope> --client-id <id>` and verify device flow prompts are displayed; store token on success.
22) Run `oauth device --provider azure-devops --scope <org> <project> --client-id <id> --tenant <tenant>` and verify device flow prompts are displayed; store OAuth token on success.
23) Run `oauth revoke --provider <provider> --scope <scope>` and verify token is removed from keyring and audit entry is logged.
24) Set `GIT_PROJECT_SYNC_OAUTH_ALLOW=github=github.com;azure-devops=dev.azure.com` and verify OAuth gating allows only listed hosts.
25) Trigger an OAuth error (expired code or access denied) and confirm troubleshooting guidance in README/SPEC aligns with the CLI error.
26) Open the TUI: verify Config and Token screens show guidance text and validation feedback when submitting empty/invalid values.
27) Build with `--features tray`, run `tray`, and verify the system tray icon shows actions for dashboard and sync; open dashboard and confirm core stats render.
28) Run `install` and confirm the daemon service/task is installed; verify delayed start when `--delayed-start` is provided.
29) Run `install --tui` and confirm the installer screen runs the same flow.
30) Run `install --path add` and confirm PATH registration message is shown.
31) Run `mirror-cli` with no args: if not installed, installer opens; if installed, help is shown.
32) Re-run `install` with a newer binary and confirm it replaces the existing install in the OS default location and restarts the service using that path.
33) Run `install --status` and confirm it prints install path, service/task state, and PATH status.
34) Run `sync --status` and confirm live progress output and final summary.
35) In the TUI dashboard, press `s` to open Sync Status and confirm current action/repo/counts are shown.
36) Run `sync` and confirm per-repo audit entries are created (e.g., `sync.repo` or `daemon.sync.repo`).
