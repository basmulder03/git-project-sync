#!/usr/bin/env bash
# Git Project Sync - Update Script for Unix-like systems
# Usage: curl -fsSL https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/update.sh | bash

set -e

# Configuration
REPO="basmulder03/git-project-sync"
BINARY_NAME="mirror-cli"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Functions
log_info() {
    echo -e "${GREEN}==>${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}Warning:${NC} $1"
}

log_error() {
    echo -e "${RED}Error:${NC} $1" >&2
}

detect_os() {
    case "$(uname -s)" in
        Linux*)     echo "linux";;
        Darwin*)    echo "macos";;
        *)          echo "unknown";;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)   echo "x86_64";;
        arm64|aarch64)  echo "aarch64";;
        *)              echo "unknown";;
    esac
}

get_current_version() {
    if command -v "$BINARY_NAME" >/dev/null 2>&1; then
        "$BINARY_NAME" --version 2>&1 | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -n1 || echo "unknown"
    else
        echo "not_installed"
    fi
}

get_latest_release() {
    curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/'
}

get_install_location() {
    if command -v "$BINARY_NAME" >/dev/null 2>&1; then
        local binary_path
        binary_path=$(command -v "$BINARY_NAME")
        dirname "$binary_path"
    else
        echo "$HOME/.local/bin"
    fi
}

download_and_update() {
    local os="$1"
    local arch="$2"
    local version="$3"
    local install_dir="$4"
    
    # Construct download URL
    local asset_name="${BINARY_NAME}-${os}-${arch}"
    if [ "$os" = "windows" ]; then
        asset_name="${asset_name}.exe"
    fi
    
    local download_url="https://github.com/${REPO}/releases/download/${version}/${asset_name}"
    
    log_info "Downloading ${BINARY_NAME} ${version}..."
    
    # Create temporary directory
    local tmp_dir
    tmp_dir=$(mktemp -d)
    trap 'rm -rf "$tmp_dir"' EXIT
    
    local tmp_file="${tmp_dir}/${BINARY_NAME}"
    
    # Download the binary
    if ! curl -fsSL -o "$tmp_file" "$download_url"; then
        log_error "Failed to download binary from $download_url"
        exit 1
    fi
    
    # Make binary executable
    chmod +x "$tmp_file"
    
    # Update binary
    log_info "Updating ${install_dir}/${BINARY_NAME}..."
    mv "$tmp_file" "${install_dir}/${BINARY_NAME}"
    
    log_info "${GREEN}Update successful!${NC}"
}

main() {
    log_info "Git Project Sync Updater"
    echo ""
    
    # Check if binary is installed
    local current_version
    current_version=$(get_current_version)
    
    if [ "$current_version" = "not_installed" ]; then
        log_error "${BINARY_NAME} is not installed"
        log_error "Please install it first using:"
        log_error "  curl -fsSL https://raw.githubusercontent.com/${REPO}/main/scripts/install.sh | bash"
        exit 1
    fi
    
    log_info "Current version: ${current_version}"
    
    # Get latest release version
    local latest_version
    log_info "Checking for updates..."
    latest_version=$(get_latest_release)
    
    if [ -z "$latest_version" ]; then
        log_error "Failed to fetch latest release version"
        exit 1
    fi
    
    # Remove 'v' prefix for comparison if present
    local latest_version_num="${latest_version#v}"
    
    log_info "Latest version: ${latest_version}"
    
    # Compare versions
    if [ "$current_version" = "$latest_version_num" ]; then
        log_info "Already up to date!"
        exit 0
    fi
    
    # Detect OS and architecture
    local os
    os=$(detect_os)
    local arch
    arch=$(detect_arch)
    
    if [ "$os" = "unknown" ] || [ "$arch" = "unknown" ]; then
        log_error "Unsupported OS or architecture: $(uname -s) $(uname -m)"
        exit 1
    fi
    
    # Get install location
    local install_dir
    install_dir=$(get_install_location)
    
    log_info "Updating from ${current_version} to ${latest_version}..."
    
    # Download and update
    download_and_update "$os" "$arch" "$latest_version" "$install_dir"
    
    # Verify update
    local new_version
    new_version=$(get_current_version)
    log_info "Updated to version: ${new_version}"
}

# Run main function
main "$@"
