#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

DEV_DIR="$REPO_ROOT/.dev/local"
DEV_CONFIG="$DEV_DIR/config.dev.yaml"
DEV_STATE_DB="$DEV_DIR/state.dev.db"

usage() {
  cat <<'EOF'
Usage: ./scripts/dev/local.sh [--refresh-config] <syncctl|syncd|synctui> [args...]

Runs local development binaries with a development copy of your current user config.

Environment overrides:
  SYNCDEV_SOURCE_CONFIG   Source config path to copy from
EOF
}

default_source_config() {
  if [[ -n "${XDG_CONFIG_HOME:-}" ]]; then
    printf "%s\n" "$XDG_CONFIG_HOME/git-project-sync/config.yaml"
    return
  fi
  printf "%s\n" "$HOME/.config/git-project-sync/config.yaml"
}

ensure_dev_config() {
  local source_config="$1"
  local refresh_config="$2"

  mkdir -p "$DEV_DIR"

  if [[ -f "$source_config" ]]; then
    if [[ "$refresh_config" == "true" || ! -f "$DEV_CONFIG" || "$source_config" -nt "$DEV_CONFIG" ]]; then
      cp "$source_config" "$DEV_CONFIG"
    fi
  elif [[ ! -f "$DEV_CONFIG" ]]; then
    cp "$REPO_ROOT/configs/config.example.yaml" "$DEV_CONFIG"
  fi

  (
    cd "$REPO_ROOT"
    go run ./cmd/syncctl --config "$DEV_CONFIG" config set state.db_path "$DEV_STATE_DB" >/dev/null
  )
}

REFRESH_CONFIG="false"
if [[ "${1:-}" == "--refresh-config" ]]; then
  REFRESH_CONFIG="true"
  shift
fi

TOOL="${1:-}"
if [[ -z "$TOOL" ]]; then
  usage
  exit 2
fi
shift

case "$TOOL" in
  syncctl|syncd|synctui)
    ;;
  *)
    usage
    exit 2
    ;;
esac

SOURCE_CONFIG="${SYNCDEV_SOURCE_CONFIG:-$(default_source_config)}"
ensure_dev_config "$SOURCE_CONFIG" "$REFRESH_CONFIG"

echo "syncdev config: $DEV_CONFIG"
echo "syncdev state:  $DEV_STATE_DB"

cd "$REPO_ROOT"
case "$TOOL" in
  syncctl)
    exec go run ./cmd/syncctl --config "$DEV_CONFIG" "$@"
    ;;
  syncd)
    exec go run ./cmd/syncd --config "$DEV_CONFIG" "$@"
    ;;
  synctui)
    exec go run ./cmd/synctui --config "$DEV_CONFIG" "$@"
    ;;
esac
