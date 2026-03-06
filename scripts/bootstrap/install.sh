#!/usr/bin/env bash
set -euo pipefail

MODE="user"
VERSION="latest"
REPO="${REPO:-basmulder03/git-project-sync}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --user)
      MODE="user"
      shift
      ;;
    --version)
      VERSION="${2:-}"
      if [[ -z "$VERSION" ]]; then
        echo "--version requires a value"
        exit 2
      fi
      shift 2
      ;;
    --repo)
      REPO="${2:-}"
      if [[ -z "$REPO" ]]; then
        echo "--repo requires a value"
        exit 2
      fi
      shift 2
      ;;
    *)
      echo "Usage: $0 [--user] [--version <tag>] [--repo <owner/name>]"
      exit 2
      ;;
  esac
done

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "this bootstrap installer supports Linux only"
  exit 1
fi

ARCH_RAW="$(uname -m)"
case "$ARCH_RAW" in
  x86_64|amd64)
    ARCH="amd64"
    ;;
  *)
    echo "unsupported architecture: $ARCH_RAW"
    exit 1
    ;;
esac

if command -v curl >/dev/null 2>&1; then
  DOWNLOADER="curl"
elif command -v wget >/dev/null 2>&1; then
  DOWNLOADER="wget"
else
  echo "missing downloader: install curl or wget"
  exit 1
fi

download() {
  local url="$1"
  local output="$2"
  if [[ "$DOWNLOADER" == "curl" ]]; then
    curl -fsSL "$url" -o "$output"
  else
    wget -qO "$output" "$url"
  fi
}

BIN_DIR="${HOME}/.local/bin"
CONFIG_PATH="${HOME}/.config/git-project-sync/config.yaml"

mkdir -p "$BIN_DIR"

if [[ "$VERSION" == "latest" ]]; then
  BASE_URL="https://github.com/${REPO}/releases/latest/download"
else
  BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

SYNC_D_FILE="$TMP_DIR/syncd_linux_${ARCH}"
SYNC_CTL_FILE="$TMP_DIR/syncctl_linux_${ARCH}"
SYNC_TUI_FILE="$TMP_DIR/synctui_linux_${ARCH}"

download "$BASE_URL/syncd_linux_${ARCH}" "$SYNC_D_FILE"
download "$BASE_URL/syncctl_linux_${ARCH}" "$SYNC_CTL_FILE"
download "$BASE_URL/synctui_linux_${ARCH}" "$SYNC_TUI_FILE"

install -m 0755 "$SYNC_D_FILE" "$BIN_DIR/syncd"
install -m 0755 "$SYNC_CTL_FILE" "$BIN_DIR/syncctl"
install -m 0755 "$SYNC_TUI_FILE" "$BIN_DIR/synctui"

ACTIVE_SYNC_D="$(command -v syncd 2>/dev/null || true)"
ACTIVE_SYNC_CTL="$(command -v syncctl 2>/dev/null || true)"
ACTIVE_SYNC_TUI="$(command -v synctui 2>/dev/null || true)"

sync_active_binary() {
  local src="$1"
  local active="$2"
  local label="$3"

  if [[ -z "$active" ]]; then
    return
  fi
  if [[ "$active" == "$BIN_DIR/$label" ]]; then
    return
  fi

  if [[ -e "$active" && -w "$active" ]]; then
    install -m 0755 "$src" "$active"
    echo "updated active $label at $active"
    return
  fi

  if [[ -w "$(dirname "$active")" ]]; then
    install -m 0755 "$src" "$active"
    echo "updated active $label at $active"
    return
  fi

  echo "warning: active $label path is not writable: $active"
}

sync_active_binary "$SYNC_D_FILE" "$ACTIVE_SYNC_D" "syncd"
sync_active_binary "$SYNC_CTL_FILE" "$ACTIVE_SYNC_CTL" "syncctl"
sync_active_binary "$SYNC_TUI_FILE" "$ACTIVE_SYNC_TUI" "synctui"

INSTALL_SCRIPT_FILE="$TMP_DIR/install.sh"
if [[ "$VERSION" == "latest" ]]; then
  INSTALL_REF="main"
else
  INSTALL_REF="$VERSION"
fi

download "https://raw.githubusercontent.com/${REPO}/${INSTALL_REF}/scripts/install.sh" "$INSTALL_SCRIPT_FILE"
chmod +x "$INSTALL_SCRIPT_FILE"

BIN_PATH="$BIN_DIR/syncd" CONFIG_PATH="$CONFIG_PATH" "$INSTALL_SCRIPT_FILE" "--$MODE"

echo "bootstrap install complete"
echo "syncd: $BIN_DIR/syncd"
echo "syncctl: $BIN_DIR/syncctl"
echo "synctui: $BIN_DIR/synctui"
echo "config: $CONFIG_PATH"
echo
echo "Next steps:"
echo "1) Validate install: $BIN_DIR/syncctl --version"
echo "2) Add a source: $BIN_DIR/syncctl source add github <source-id> --account <account>"
echo "3) Login PAT: $BIN_DIR/syncctl auth login <source-id> --token <pat>"
echo "4) Register repos: $BIN_DIR/syncctl repo add <path> --source-id <source-id>"
echo "5) Dry-run first sync: $BIN_DIR/syncctl sync all --dry-run"
echo "6) Monitor health: $BIN_DIR/syncctl doctor && $BIN_DIR/syncctl daemon status"
echo
echo "See docs/getting-started/first-run-onboarding.md for guided onboarding."
