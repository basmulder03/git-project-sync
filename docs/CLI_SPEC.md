# CLI Spec (v1)

Binary name: `syncctl`

## Global

- `syncctl --help`
- `syncctl --version`
- `syncctl doctor`

## Source Management

- `syncctl source add github <source-id> [--account <name>] [--org <name>]`
- `syncctl source add azure <source-id> [--account <name>] [--org <name>]`
- `syncctl source remove <source-id>`
- `syncctl source list`
- `syncctl source show <source-id>`

## Repo Management

- `syncctl repo add <path> [--remote origin]`
- `syncctl repo clone <source-id> <repo-slug> [--into managed]`
- `syncctl repo remove <path>`
- `syncctl repo list`
- `syncctl repo show <path>`
- `syncctl repo sync <path> [--dry-run]`

## Workspace

- `syncctl workspace show`
- `syncctl workspace set-root <path>`
- `syncctl workspace layout check`
- `syncctl workspace layout fix [--dry-run]`

## Sync Actions

- `syncctl sync all [--dry-run]`
- `syncctl sync repo <path> [--dry-run]`

## Daemon Control

- `syncctl daemon start`
- `syncctl daemon stop`
- `syncctl daemon restart`
- `syncctl daemon status`
- `syncctl daemon logs [--follow] [--limit N]`

## Configuration

- `syncctl config init`
- `syncctl config show`
- `syncctl config get <key>`
- `syncctl config set <key> <value>`
- `syncctl config validate`

## Credentials

- `syncctl auth login <source-id>`
- `syncctl auth test <source-id>`
- `syncctl auth logout <source-id>`

## Cache

- `syncctl cache show`
- `syncctl cache refresh [providers|branches|all]`
- `syncctl cache clear [providers|branches|all]`

## Stats

- `syncctl stats show`
- `syncctl events list [--limit N]`
- `syncctl trace show <trace-id>`

## Install and Service

- `syncctl install --user|--system`
- `syncctl uninstall --user|--system`
- `syncctl service register`
- `syncctl service unregister`

## Update

- `syncctl update check`
- `syncctl update apply [--channel stable]`
