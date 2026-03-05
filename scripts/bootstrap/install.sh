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
    --system)
      MODE="system"
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
      echo "Usage: $0 [--user|--system] [--version <tag>] [--repo <owner/name>]"
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

if [[ "$MODE" == "system" && "$(id -u)" -ne 0 ]]; then
  echo "system bootstrap install requires root"
  exit 1
fi

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

if [[ "$MODE" == "system" ]]; then
  BIN_DIR="/usr/local/bin"
  CONFIG_PATH="/etc/git-project-sync/config.yaml"
else
  BIN_DIR="${HOME}/.local/bin"
  CONFIG_PATH="${HOME}/.config/git-project-sync/config.yaml"
fi

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

download "$BASE_URL/syncd_linux_${ARCH}" "$SYNC_D_FILE"
download "$BASE_URL/syncctl_linux_${ARCH}" "$SYNC_CTL_FILE"

install -m 0755 "$SYNC_D_FILE" "$BIN_DIR/syncd"
install -m 0755 "$SYNC_CTL_FILE" "$BIN_DIR/syncctl"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

BIN_PATH="$BIN_DIR/syncd" CONFIG_PATH="$CONFIG_PATH" "$REPO_ROOT/scripts/install.sh" "--$MODE"

echo "bootstrap install complete"
echo "syncd: $BIN_DIR/syncd"
echo "syncctl: $BIN_DIR/syncctl"
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
echo "See docs/QUICKSTART.md for guided onboarding."
