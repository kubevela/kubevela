#!/usr/bin/env bash

# This script downloads and installs kubebuilder

set -e

# Constants
INSTALL_DIR=${INSTALL_DIR:-/usr/local/kubebuilder}
BINARIES_DIR=${KUBEBUILDER_ASSETS:-${INSTALL_DIR}/bin}
KUBEBUILDER_VERSION=${KUBEBUILDER_VERSION:-3.12.0}
OS=$(uname | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
if [ "${ARCH}" == "x86_64" ]; then
  ARCH="amd64"
fi

echo "==> Installing kubebuilder version ${KUBEBUILDER_VERSION} to ${INSTALL_DIR}"
echo "OS: ${OS}, ARCH: ${ARCH}"

# Create install directory if it doesn't exist
if [ ! -d "${INSTALL_DIR}" ]; then
  echo "==> Creating directory ${INSTALL_DIR}"
  mkdir -p "${INSTALL_DIR}" || sudo mkdir -p "${INSTALL_DIR}"
fi

# Ensure install directory is writable
if [ ! -w "${INSTALL_DIR}" ]; then
  echo "==> Making ${INSTALL_DIR} writable with sudo"
  sudo chmod -R 777 "${INSTALL_DIR}"
fi

# Function to download a file with progress
download_with_progress() {
    local url=$1
    local output=$2
    echo "Downloading $url to $output"
    curl -L --progress-bar --retry 5 --retry-delay 3 "$url" -o "$output"
}

# Function to download and install kubebuilder
install_kubebuilder() {
    echo "==> Downloading kubebuilder version ${KUBEBUILDER_VERSION}"
    
    local TMP_DIR=$(mktemp -d)
    pushd "${TMP_DIR}" > /dev/null
    
    local KUBEBUILDER_URL="https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${KUBEBUILDER_VERSION}/kubebuilder_${OS}_${ARCH}"
    download_with_progress "${KUBEBUILDER_URL}" "kubebuilder"
    
    # Create bin directory if it doesn't exist
    if [ ! -d "${BINARIES_DIR}" ]; then
      echo "==> Creating directory ${BINARIES_DIR}"
      mkdir -p "${BINARIES_DIR}" || sudo mkdir -p "${BINARIES_DIR}"
    fi
    
    # Copy binary to the target directory
    echo "==> Installing kubebuilder binary to ${BINARIES_DIR}"
    cp kubebuilder "${BINARIES_DIR}/kubebuilder" || sudo cp kubebuilder "${BINARIES_DIR}/kubebuilder"
    
    # Make binary executable
    chmod +x "${BINARIES_DIR}/kubebuilder" || sudo chmod +x "${BINARIES_DIR}/kubebuilder"
    
    popd > /dev/null
    rm -rf "${TMP_DIR}"
    
    echo "✓ kubebuilder installed successfully"
}

# Check for kubebuilder
if [ ! -f "${BINARIES_DIR}/kubebuilder" ] || [ ! -x "${BINARIES_DIR}/kubebuilder" ]; then
    echo "kubebuilder not found or not executable, downloading..."
    install_kubebuilder
else
    echo "✓ kubebuilder already installed at ${BINARIES_DIR}/kubebuilder"
fi

# Set environment variables
echo "==> Setting environment variables"
export PATH="${BINARIES_DIR}:${PATH}"

echo "==> kubebuilder installation complete"
echo "PATH=${PATH}"

# Verify kubebuilder is accessible
echo "==> Verifying kubebuilder is accessible"
ls -la "${BINARIES_DIR}/kubebuilder"
${BINARIES_DIR}/kubebuilder version || echo "WARNING: kubebuilder not working properly"
