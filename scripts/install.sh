#!/usr/bin/env bash
# Git Project Sync - Installation Script for Unix-like systems
# Usage: curl -fsSL https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/install.sh | bash
# Or: bash <(curl -fsSL https://raw.githubusercontent.com/basmulder03/git-project-sync/main/scripts/install.sh)

set -e

# Configuration
REPO="basmulder03/git-project-sync"
BINARY_NAME="mirror-cli"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

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

get_latest_release() {
    curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/'
}

download_and_install() {
    local os="$1"
    local arch="$2"
    local version="$3"
    
    # Construct download URL
    local asset_name="${BINARY_NAME}-${os}-${arch}"
    if [ "$os" = "windows" ]; then
        asset_name="${asset_name}.exe"
    fi
    
    local download_url="https://github.com/${REPO}/releases/download/${version}/${asset_name}"
    
    log_info "Downloading ${BINARY_NAME} ${version} for ${os}-${arch}..."
    log_info "URL: ${download_url}"
    
    # Create temporary directory
    local tmp_dir
    tmp_dir=$(mktemp -d)
    trap 'rm -rf "$tmp_dir"' EXIT
    
    local tmp_file="${tmp_dir}/${BINARY_NAME}"
    
    # Download the binary
    if ! curl -fsSL -o "$tmp_file" "$download_url"; then
        log_error "Failed to download binary from $download_url"
        log_error "Please check if the release exists and the asset is available"
        exit 1
    fi
    
    # Make binary executable
    chmod +x "$tmp_file"
    
    # Create install directory if it doesn't exist
    mkdir -p "$INSTALL_DIR"
    
    # Install binary
    log_info "Installing to ${INSTALL_DIR}/${BINARY_NAME}..."
    mv "$tmp_file" "${INSTALL_DIR}/${BINARY_NAME}"
    
    log_info "${GREEN}Installation successful!${NC}"
}

verify_installation() {
    if command -v "$BINARY_NAME" >/dev/null 2>&1; then
        log_info "✓ ${BINARY_NAME} is now available in your PATH"
        log_info "Version: $("$BINARY_NAME" --version 2>&1 || echo 'Unable to get version')"
    else
        log_warn "${BINARY_NAME} was installed but is not in your PATH"
        log_warn "Add ${INSTALL_DIR} to your PATH by adding this to your shell profile:"
        log_warn "  export PATH=\"${INSTALL_DIR}:\$PATH\""
        log_warn ""
        log_warn "Or run directly: ${INSTALL_DIR}/${BINARY_NAME}"
    fi
}

show_next_steps() {
    echo ""
    log_info "Next steps:"
    echo "  1. Initialize config: ${BINARY_NAME} config init --root /path/to/mirrors"
    echo "  2. Add a target: ${BINARY_NAME} target add --provider github --scope your-org"
    echo "  3. Set token: ${BINARY_NAME} token set --provider github --scope your-org --token YOUR_TOKEN"
    echo "  4. Run sync: ${BINARY_NAME} sync"
    echo ""
    echo "For more information, visit: https://github.com/${REPO}"
}

main() {
    log_info "Git Project Sync Installer"
    echo ""
    
    # Detect OS and architecture
    local os
    os=$(detect_os)
    local arch
    arch=$(detect_arch)
    
    if [ "$os" = "unknown" ] || [ "$arch" = "unknown" ]; then
        log_error "Unsupported OS or architecture: $(uname -s) $(uname -m)"
        log_error "Please build from source using: cargo build --release"
        exit 1
    fi
    
    log_info "Detected: ${os}-${arch}"
    
    # Get latest release version
    local version
    log_info "Fetching latest release..."
    version=$(get_latest_release)
    
    if [ -z "$version" ]; then
        log_error "Failed to fetch latest release version"
        log_error "Please check your internet connection and try again"
        exit 1
    fi
    
    log_info "Latest version: ${version}"
    
    # Download and install
    download_and_install "$os" "$arch" "$version"
    
    # Verify installation
    verify_installation
    
    # Show next steps
    show_next_steps
}

# Run main function
main "$@"
