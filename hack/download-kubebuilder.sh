#!/usr/bin/env bash

# This script downloads and installs kubebuilder

set -e

# Constants
KUBEBUILDER_VERSION=${KUBEBUILDER_VERSION:-3.12.0}
INSTALL_DIR=${INSTALL_DIR:-/usr/local/kubebuilder}
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

echo "==> Downloading kubebuilder version ${KUBEBUILDER_VERSION}"

TMP_DIR=$(mktemp -d)
pushd "${TMP_DIR}" > /dev/null

KUBEBUILDER_URL="https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${KUBEBUILDER_VERSION}/kubebuilder_${OS}_${ARCH}"
download_with_progress "${KUBEBUILDER_URL}" "kubebuilder"

# Create bin directory
mkdir -p "${INSTALL_DIR}/bin"

# Copy binary to the target directory
echo "==> Installing kubebuilder binary to ${INSTALL_DIR}/bin"
cp kubebuilder "${INSTALL_DIR}/bin/kubebuilder" || sudo cp kubebuilder "${INSTALL_DIR}/bin/kubebuilder"

# Make binary executable
chmod +x "${INSTALL_DIR}/bin/kubebuilder" || sudo chmod +x "${INSTALL_DIR}/bin/kubebuilder"

popd > /dev/null
rm -rf "${TMP_DIR}"

echo "✓ kubebuilder installed successfully"

# Set environment variables
echo "==> Setting environment variables"
export PATH="${INSTALL_DIR}/bin:${PATH}"
chmod +x hack/download-kubebuilder.sh
echo "==> kubebuilder installation complete"
echo "PATH=${PATH}"

# Verify binary is accessible
echo "==> Verifying kubebuilder is accessible"
${INSTALL_DIR}/bin/kubebuilder version || echo "WARNING: kubebuilder not working properly"
