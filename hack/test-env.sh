#!/usr/bin/env bash

# This script verifies and prepares the test environment

set -e

KUBEBUILDER_DIR=${KUBEBUILDER_DIR:-/usr/local/kubebuilder}
KUBEBUILDER_BIN=${KUBEBUILDER_BIN:-${KUBEBUILDER_DIR}/bin}

function check_binary() {
  local binary=$1
  local message=$2
  
  if [ ! -f "${KUBEBUILDER_BIN}/${binary}" ]; then
    echo "ERROR: ${binary} not found at ${KUBEBUILDER_BIN}/${binary}"
    echo "${message}"
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

echo "===========> Verifying test environment"
echo "KUBEBUILDER_DIR: ${KUBEBUILDER_DIR}"
echo "KUBEBUILDER_BIN: ${KUBEBUILDER_BIN}"

# Check if directory exists and is writable
if [ ! -d "${KUBEBUILDER_BIN}" ]; then
  echo "Creating directory ${KUBEBUILDER_BIN}"
  mkdir -p "${KUBEBUILDER_BIN}" || sudo mkdir -p "${KUBEBUILDER_BIN}"
fi

if [ ! -w "${KUBEBUILDER_BIN}" ]; then
  echo "Directory ${KUBEBUILDER_BIN} is not writable, attempting to fix permissions"
  sudo chmod -R 777 "${KUBEBUILDER_BIN}" || true
fi

# Check for required binaries
check_binary "kubebuilder" "Run 'make setup-kubebuilder' to install" || EXIT_CODE=1
check_binary "etcd" "Run 'make setup-kubebuilder' to install" || EXIT_CODE=1
check_binary "etcdctl" "Run 'make setup-kubebuilder' to install" || EXIT_CODE=1

if [ -n "$EXIT_CODE" ]; then
  echo "===========> Test environment verification FAILED"
  echo "Please run 'make setup-kubebuilder' to install missing components"
  exit 1
else
  echo "===========> Test environment verification PASSED"
  echo "PATH: $PATH"
  echo "Adding ${KUBEBUILDER_BIN} to PATH if not already present"
  export PATH="${KUBEBUILDER_BIN}:$PATH"
  export KUBEBUILDER_ASSETS="${KUBEBUILDER_BIN}"
  echo "KUBEBUILDER_ASSETS: ${KUBEBUILDER_ASSETS}"
fi
