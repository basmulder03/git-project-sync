#!/usr/bin/env bash
set -euo pipefail

MODE="user"
if [[ "${1:-}" == "--user" || -z "${1:-}" ]]; then
  MODE="user"
else
  echo "Usage: $0 [--user]"
  exit 2
fi

BIN_PATH="${BIN_PATH:-$HOME/.local/bin/syncd}"
CONFIG_PATH="${CONFIG_PATH:-$HOME/.config/git-project-sync/config.yaml}"

if [[ ! -x "$BIN_PATH" ]]; then
  echo "syncd binary not found or not executable at $BIN_PATH"
  exit 1
fi

mkdir -p "$(dirname "$CONFIG_PATH")"
if [[ ! -f "$CONFIG_PATH" ]]; then
  cat >"$CONFIG_PATH" <<EOF
daemon:
  interval: 5m
repositories: []
sources: []
EOF
fi

SERVICE_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
SYSTEMCTL=(systemctl --user)

mkdir -p "$SERVICE_DIR"
SERVICE_FILE="$SERVICE_DIR/git-project-sync.service"

cat >"$SERVICE_FILE" <<EOF
[Unit]
Description=Git Project Sync daemon
After=network-online.target

[Service]
Type=simple
ExecStart=$BIN_PATH --config $CONFIG_PATH
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
EOF

"${SYSTEMCTL[@]}" daemon-reload
"${SYSTEMCTL[@]}" enable --now git-project-sync.service
echo "installed git-project-sync service in $MODE mode"
