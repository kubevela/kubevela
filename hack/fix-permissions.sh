#!/usr/bin/env bash

# This script fixes permissions for kubebuilder binaries

set -o errexit
set -o nounset
set -o pipefail

# Determine script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Default kubebuilder bin directory
KUBEBUILDER_BIN_DIR="${KUBEBUILDER_ASSETS:-/usr/local/kubebuilder/bin}"
mkdir -p "${KUBEBUILDER_BIN_DIR}"

echo "Fixing permissions for kubebuilder binaries in ${KUBEBUILDER_BIN_DIR}"

# Fix directory permissions
sudo chmod 755 "${KUBEBUILDER_BIN_DIR}"
sudo chown -R "$(whoami)" "${KUBEBUILDER_BIN_DIR}"

# Fix binary permissions
BINARIES=("etcd" "kube-apiserver" "kubectl")
for binary in "${BINARIES[@]}"; do
  BINARY_PATH="${KUBEBUILDER_BIN_DIR}/${binary}"
  if [[ -f "${BINARY_PATH}" ]]; then
    echo "Setting executable permissions for ${binary}"
    chmod +x "${BINARY_PATH}"
    
    # Verify permissions
    if [[ -x "${BINARY_PATH}" ]]; then
      echo "✓ ${binary} is now executable"
    else
      echo "✗ Failed to make ${binary} executable"
      exit 1
    fi
  else
    echo "Binary ${binary} not found at ${BINARY_PATH}"
  fi
done

echo "Permission fix complete!"
