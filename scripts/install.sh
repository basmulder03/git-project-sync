#!/usr/bin/env bash
set -euo pipefail

MODE="user"
if [[ "${1:-}" == "--system" ]]; then
  MODE="system"
elif [[ "${1:-}" == "--user" || -z "${1:-}" ]]; then
  MODE="user"
else
  echo "Usage: $0 [--user|--system]"
  exit 2
fi

if [[ "$MODE" == "system" && "$(id -u)" -ne 0 ]]; then
  echo "system install requires root"
  exit 1
fi

BIN_PATH="${BIN_PATH:-/usr/local/bin/syncd}"
CONFIG_PATH="${CONFIG_PATH:-$HOME/.config/git-project-sync/config.yaml}"

if [[ "$MODE" == "user" ]]; then
  SERVICE_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
  SYSTEMCTL=(systemctl --user)
else
  SERVICE_DIR="/etc/systemd/system"
  SYSTEMCTL=(systemctl)
fi

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
