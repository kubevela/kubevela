#!/usr/bin/env bash

# This script serves as a wrapper for running tests with the proper environment variables

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

KUBEBUILDER_DIR=${KUBEBUILDER_DIR:-/usr/local/kubebuilder}
KUBEBUILDER_BIN=${KUBEBUILDER_BIN:-${KUBEBUILDER_DIR}/bin}
KUBEBUILDER_ASSETS=${KUBEBUILDER_ASSETS:-${KUBEBUILDER_BIN}}

# Check if test binaries exist
function check_binary() {
  local binary=$1
  if [ ! -f "${KUBEBUILDER_BIN}/${binary}" ]; then
    echo "ERROR: ${binary} not found at ${KUBEBUILDER_BIN}/${binary}"
    return 1
  else
    echo "✓ ${binary} found at ${KUBEBUILDER_BIN}/${binary}"
    if [ ! -x "${KUBEBUILDER_BIN}/${binary}" ]; then
      echo "WARNING: ${binary} is not executable, fixing permissions"
      chmod +x "${KUBEBUILDER_BIN}/${binary}" || sudo chmod +x "${KUBEBUILDER_BIN}/${binary}"
    fi
    return 0
  fi
}

# Verify binaries exist
echo "===========> Verifying test environment"
if ! check_binary "etcd"; then
  echo "Setting up kubebuilder environment..."
  make -C "${ROOT_DIR}" setup-kubebuilder
fi

if ! check_binary "kubebuilder"; then
  echo "Setting up kubebuilder environment..."
  make -C "${ROOT_DIR}" setup-kubebuilder
fi

# Export environment variables
export PATH="${KUBEBUILDER_BIN}:${PATH}"
export KUBEBUILDER_ASSETS="${KUBEBUILDER_BIN}"

echo "===========> Running tests with environment:"
echo "PATH: ${PATH}"
echo "KUBEBUILDER_ASSETS: ${KUBEBUILDER_ASSETS}"
echo "Test command: $*"

# Run the test command with the proper environment
exec "$@"
