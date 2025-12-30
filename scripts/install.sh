#!/bin/bash
# CertWatch Agent Installation Script
# Usage: curl -sSL https://get.certwatch.app | bash
#    or: curl -sSL https://raw.githubusercontent.com/certwatch-app/cw-agent/main/scripts/install.sh | bash

set -e

# Configuration
REPO="certwatch-app/cw-agent"
BINARY_NAME="cw-agent"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Print functions
info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

# Detect OS and architecture
detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    case $ARCH in
        x86_64|amd64)
            ARCH="amd64"
            ;;
        aarch64|arm64)
            ARCH="arm64"
            ;;
        *)
            error "Unsupported architecture: $ARCH"
            ;;
    esac

    case $OS in
        linux)
            OS="linux"
            ;;
        darwin)
            OS="darwin"
            ;;
        mingw*|msys*|cygwin*|windows*)
            OS="windows"
            ;;
        *)
            error "Unsupported OS: $OS"
            ;;
    esac

    info "Detected platform: ${OS}/${ARCH}"
}

# Get latest version from GitHub
get_latest_version() {
    info "Fetching latest version..."
    VERSION=$(curl -sL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

    if [ -z "$VERSION" ]; then
        error "Failed to get latest version. Please check your internet connection."
    fi

    info "Latest version: ${VERSION}"
}

# Download and install
install() {
    # Determine file extension
    EXT="tar.gz"
    if [ "$OS" = "windows" ]; then
        EXT="zip"
    fi

    # Build download URL
    FILENAME="${BINARY_NAME}_${VERSION#v}_${OS}_${ARCH}.${EXT}"
    URL="https://github.com/${REPO}/releases/download/${VERSION}/${FILENAME}"

    info "Downloading ${URL}..."

    # Create temp directory
    TMP_DIR=$(mktemp -d)
    trap "rm -rf ${TMP_DIR}" EXIT

    # Download
    if command -v curl &> /dev/null; then
        curl -sL "${URL}" -o "${TMP_DIR}/${FILENAME}"
    elif command -v wget &> /dev/null; then
        wget -q "${URL}" -O "${TMP_DIR}/${FILENAME}"
    else
        error "Neither curl nor wget found. Please install one of them."
    fi

    # Extract
    info "Extracting..."
    cd "${TMP_DIR}"
    if [ "$EXT" = "tar.gz" ]; then
        tar -xzf "${FILENAME}"
    else
        unzip -q "${FILENAME}"
    fi

    # Install
    info "Installing to ${INSTALL_DIR}..."

    # Check if we need sudo
    if [ -w "${INSTALL_DIR}" ]; then
        mv "${BINARY_NAME}" "${INSTALL_DIR}/"
    else
        warn "Need sudo to install to ${INSTALL_DIR}"
        sudo mv "${BINARY_NAME}" "${INSTALL_DIR}/"
    fi

    chmod +x "${INSTALL_DIR}/${BINARY_NAME}"

    info "Installation complete!"
}

# Verify installation
verify() {
    if command -v ${BINARY_NAME} &> /dev/null; then
        info "Verifying installation..."
        ${BINARY_NAME} version
        echo ""
        info "CertWatch Agent installed successfully!"
        info ""
        info "Next steps:"
        info "  1. Create a config file: cp /path/to/certwatch.example.yaml certwatch.yaml"
        info "  2. Add your API key and certificates to certwatch.yaml"
        info "  3. Start the agent: ${BINARY_NAME} start -c certwatch.yaml"
        info ""
        info "Documentation: https://certwatch.app/docs/agent"
    else
        error "Installation verification failed. ${BINARY_NAME} not found in PATH."
    fi
}

# Main
main() {
    echo "========================================"
    echo "  CertWatch Agent Installer"
    echo "========================================"
    echo ""

    detect_platform
    get_latest_version
    install
    verify
}

main
