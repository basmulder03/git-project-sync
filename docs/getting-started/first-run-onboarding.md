# Quickstart (First Run)

This walkthrough gets a fresh install to the first successful sync cycle.

## 1) Confirm binaries and service registration

Run:

```bash
syncctl --version
syncctl doctor
syncctl daemon status
```

If `syncctl` is not on `PATH`, use the full install path shown by the bootstrap installer output.

## 2) Add provider sources

GitHub example:

```bash
syncctl source add github gh-personal --account jane-doe
```

Azure DevOps example:

```bash
syncctl source add azure az-work --account org-name
```

List configured sources:

```bash
syncctl source list
```

## 3) Authenticate with PAT

```bash
syncctl auth login gh-personal --token <github_pat>
syncctl auth login az-work --token <azure_pat>
```

Validate authentication:

```bash
syncctl auth test gh-personal
syncctl auth test az-work
```

## 4) Register repositories

```bash
syncctl repo add /path/to/repo-a --source-id gh-personal
syncctl repo add /path/to/repo-b --source-id az-work
syncctl repo list
```

## 5) Run first sync safely

Dry run first:

```bash
syncctl sync all --dry-run
```

Then apply:

```bash
syncctl sync all
```

## 6) Verify health and traceability

```bash
syncctl doctor
syncctl stats show
syncctl events list --limit 50
```

If something fails, inspect one trace:

```bash
syncctl trace show <trace-id>
```

## 7) Optional: use the TUI views

- Runtime dashboard: `synctui`
- Installer/repair dashboard: `syncsetup`

## Notes

- Sync mutations are skipped on dirty repositories by design.
- Use `syncctl state backup --output <path>` before high-risk changes.
- See `docs/getting-started/installation-and-service-registration.md` for install/repair flows and mode-specific behavior.
