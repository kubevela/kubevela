#!/usr/bin/env bash

# This script downloads and installs kubebuilder and required test binaries

set -e

# Constants
KUBEBUILDER_VERSION=${KUBEBUILDER_VERSION:-3.4.0}
INSTALL_DIR=${INSTALL_DIR:-/usr/local/kubebuilder}
OS=$(uname | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
if [ "${ARCH}" == "x86_64" ]; then
  ARCH="amd64"
fi

echo "==> Installing kubebuilder v${KUBEBUILDER_VERSION} to ${INSTALL_DIR}"
echo "OS: ${OS}, ARCH: ${ARCH}"

# Create install directory if it doesn't exist
if [ ! -d "${INSTALL_DIR}" ]; then
  echo "==> Creating directory ${INSTALL_DIR}"
  mkdir -p "${INSTALL_DIR}" || sudo mkdir -p "${INSTALL_DIR}"
fi

# Ensure bin directory exists
if [ ! -d "${INSTALL_DIR}/bin" ]; then
  echo "==> Creating bin directory ${INSTALL_DIR}/bin"
  mkdir -p "${INSTALL_DIR}/bin" || sudo mkdir -p "${INSTALL_DIR}/bin"
fi

# Make sure the directory is writable
chmod -R 777 "${INSTALL_DIR}/bin" 2>/dev/null || sudo chmod -R 777 "${INSTALL_DIR}/bin"

# Download kubebuilder binary
KUBEBUILDER_BIN="${INSTALL_DIR}/bin/kubebuilder"
if [ ! -f "${KUBEBUILDER_BIN}" ]; then
  echo "==> Downloading kubebuilder binary"
  
  # Create temp directory
  TMP_DIR=$(mktemp -d)
  cd "${TMP_DIR}"
  
  # Download kubebuilder
  DOWNLOAD_URL="https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${KUBEBUILDER_VERSION}/kubebuilder_${KUBEBUILDER_VERSION}_${OS}_${ARCH}.tar.gz"
  echo "==> Downloading kubebuilder from ${DOWNLOAD_URL}"
  curl -sL --retry 5 --retry-delay 3 "${DOWNLOAD_URL}" | tar -xz
  
  # Copy kubebuilder binary
  cp kubebuilder_${KUBEBUILDER_VERSION}_${OS}_${ARCH}/bin/kubebuilder "${INSTALL_DIR}/bin/" || \
    sudo cp kubebuilder_${KUBEBUILDER_VERSION}_${OS}_${ARCH}/bin/kubebuilder "${INSTALL_DIR}/bin/"
  
  # Make binary executable
  chmod +x "${INSTALL_DIR}/bin/kubebuilder" || sudo chmod +x "${INSTALL_DIR}/bin/kubebuilder"
  
  # Clean up temp directory
  cd - > /dev/null
  rm -rf "${TMP_DIR}"
  
  echo "✓ kubebuilder binary installed at ${INSTALL_DIR}/bin/kubebuilder"
else
  echo "✓ kubebuilder binary already exists at ${INSTALL_DIR}/bin/kubebuilder"
fi

# Call the download-test-binaries script to ensure all required binaries are present
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [ -f "${SCRIPT_DIR}/download-test-binaries.sh" ]; then
  echo "==> Running download-test-binaries.sh to get required test binaries"
  chmod +x "${SCRIPT_DIR}/download-test-binaries.sh"
  KUBEBUILDER_ASSETS="${INSTALL_DIR}/bin" "${SCRIPT_DIR}/download-test-binaries.sh"
else
  echo "ERROR: download-test-binaries.sh not found in ${SCRIPT_DIR}"
  exit 1
fi

# Set KUBEBUILDER_ASSETS environment variable for the current session
export KUBEBUILDER_ASSETS=${INSTALL_DIR}/bin
echo "KUBEBUILDER_ASSETS is set to ${KUBEBUILDER_ASSETS}"

# Verify the installation
echo "==> Verifying installation"

# Check for kubebuilder
if [ -x "${INSTALL_DIR}/bin/kubebuilder" ]; then
  echo "✓ kubebuilder installed at ${INSTALL_DIR}/bin/kubebuilder"
else
  echo "ERROR: kubebuilder not found or not executable at ${INSTALL_DIR}/bin/kubebuilder"
  exit 1
fi

# Check for etcd
if [ -x "${INSTALL_DIR}/bin/etcd" ]; then
  echo "✓ etcd installed at ${INSTALL_DIR}/bin/etcd"
else
  echo "ERROR: etcd not found or not executable at ${INSTALL_DIR}/bin/etcd"
  exit 1
fi

# Check for kube-apiserver
if [ -x "${INSTALL_DIR}/bin/kube-apiserver" ]; then
  echo "✓ kube-apiserver installed at ${INSTALL_DIR}/bin/kube-apiserver"
else
  echo "ERROR: kube-apiserver not found or not executable at ${INSTALL_DIR}/bin/kube-apiserver"
  exit 1
fi

echo "==> Installation complete!"
echo "Add ${INSTALL_DIR}/bin to your PATH to use the installed binaries:"
echo "export PATH=\$PATH:${INSTALL_DIR}/bin"
echo "export KUBEBUILDER_ASSETS=${INSTALL_DIR}/bin"
