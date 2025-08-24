#!/usr/bin/env bash

# This script fixes issues related to kubebuilder binaries

set -o errexit
set -o nounset
set -o pipefail

# Determine script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Default kubebuilder bin directory
KUBEBUILDER_BIN_DIR="${KUBEBUILDER_ASSETS:-/usr/local/kubebuilder/bin}"

echo "================ KUBEBUILDER FIX ================"
echo "Checking kubebuilder binaries..."

# Check if required binaries exist
MISSING_BINARIES=false
for binary in etcd kube-apiserver kubectl; do
  if [[ ! -x "${KUBEBUILDER_BIN_DIR}/${binary}" ]]; then
    echo "✗ Binary ${binary} is missing or not executable"
    MISSING_BINARIES=true
  else
    echo "✓ Binary ${binary} exists and is executable"
  fi
done

# Download binaries if needed
if [[ "${MISSING_BINARIES}" == "true" ]]; then
  echo ""
  echo "Some required binaries are missing. Downloading them now..."
  "${ROOT_DIR}/hack/download-binaries.sh"
else
  echo ""
  echo "All required binaries are present."
fi

# Ensure KUBEBUILDER_ASSETS is set in the environment
if [[ -z "${KUBEBUILDER_ASSETS:-}" ]]; then
  echo ""
  echo "KUBEBUILDER_ASSETS environment variable is not set"
  echo "Setting KUBEBUILDER_ASSETS=${KUBEBUILDER_BIN_DIR}"
  export KUBEBUILDER_ASSETS="${KUBEBUILDER_BIN_DIR}"
fi

# Fix permissions if needed
if [[ ! -w "${KUBEBUILDER_BIN_DIR}" ]]; then
  echo ""
  echo "Fixing permissions on ${KUBEBUILDER_BIN_DIR}..."
  sudo chown "$(whoami)" "${KUBEBUILDER_BIN_DIR}"
  sudo chmod 755 "${KUBEBUILDER_BIN_DIR}"
fi

# Make binaries executable
for binary in etcd kube-apiserver kubectl; do
  if [[ -f "${KUBEBUILDER_BIN_DIR}/${binary}" ]]; then
    sudo chmod +x "${KUBEBUILDER_BIN_DIR}/${binary}"
  fi
done

echo ""
echo "Kubebuilder fix completed!"
echo "KUBEBUILDER_ASSETS=${KUBEBUILDER_ASSETS}"
