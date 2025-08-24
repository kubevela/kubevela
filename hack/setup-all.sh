#!/usr/bin/env bash

# This script sets up everything needed to run tests

set -o errexit
set -o nounset
set -o pipefail

# Determine script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Step 1: Download binaries
echo "=== Step 1: Downloading Kubebuilder binaries ==="
"${ROOT_DIR}/hack/download-binaries.sh"
echo ""

# Step 2: Fix permissions
echo "=== Step 2: Fixing binary permissions ==="
"${ROOT_DIR}/hack/fix-permissions.sh"
echo ""

# Step 3: Set environment variables
echo "=== Step 3: Setting environment variables ==="
export KUBEBUILDER_ASSETS="${KUBEBUILDER_ASSETS:-/usr/local/kubebuilder/bin}"
echo "Set KUBEBUILDER_ASSETS=${KUBEBUILDER_ASSETS}"

# Add to .envrc if it exists or create it
if [[ -f "${ROOT_DIR}/.envrc" ]]; then
  if ! grep -q "KUBEBUILDER_ASSETS" "${ROOT_DIR}/.envrc"; then
    echo "export KUBEBUILDER_ASSETS=${KUBEBUILDER_ASSETS}" >> "${ROOT_DIR}/.envrc"
    echo "Added KUBEBUILDER_ASSETS to .envrc"
  fi
else
  echo "export KUBEBUILDER_ASSETS=${KUBEBUILDER_ASSETS}" > "${ROOT_DIR}/.envrc"
  echo "Created .envrc with KUBEBUILDER_ASSETS"
fi

# Step 4: Verify setup
echo "=== Step 4: Verifying setup ==="
for binary in etcd kube-apiserver kubectl; do
  BINARY_PATH="${KUBEBUILDER_ASSETS}/${binary}"
  if [[ -x "${BINARY_PATH}" ]]; then
    VERSION=$("${BINARY_PATH}" --version 2>&1 || echo "Version check failed")
    echo "✓ ${binary}: ${VERSION}"
  else
    echo "✗ ${binary} is missing or not executable"
    exit 1
  fi
done

echo ""
echo "Setup completed successfully!"
echo "To use in your current shell, run:"
echo "export KUBEBUILDER_ASSETS=${KUBEBUILDER_ASSETS}"
