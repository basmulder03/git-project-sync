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
  echo "system uninstall requires root"
  exit 1
fi

if [[ "$MODE" == "user" ]]; then
  SERVICE_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
  SYSTEMCTL=(systemctl --user)
else
  SERVICE_DIR="/etc/systemd/system"
  SYSTEMCTL=(systemctl)
fi

SERVICE_FILE="$SERVICE_DIR/git-project-sync.service"

"${SYSTEMCTL[@]}" disable --now git-project-sync.service || true
"${SYSTEMCTL[@]}" daemon-reload || true
rm -f "$SERVICE_FILE"

echo "uninstalled git-project-sync service in $MODE mode"
