#!/usr/bin/env bash

# This script runs tests locally with proper kubebuilder setup

set -o errexit
set -o nounset
set -o pipefail

# Determine script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Set up kubebuilder environment
echo "Setting up kubebuilder environment..."
"${ROOT_DIR}/hack/setup-kubebuilder.sh"

# Export KUBEBUILDER_ASSETS
export KUBEBUILDER_ASSETS="${KUBEBUILDER_ASSETS:-/usr/local/kubebuilder/bin}"
echo "Using KUBEBUILDER_ASSETS=${KUBEBUILDER_ASSETS}"

# Verify binaries are executable
for binary in etcd kube-apiserver kubectl; do
  if [[ ! -x "${KUBEBUILDER_ASSETS}/${binary}" ]]; then
    echo "Error: ${binary} is missing or not executable"
    exit 1
  fi
done

# Parse command-line arguments
PACKAGES=()
VERBOSE=false
RACE=false
UPDATE=false
COVER=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    -v|--verbose)
      VERBOSE=true
      shift
      ;;
    -r|--race)
      RACE=true
      shift
      ;;
    -u|--update)
      UPDATE=true
      shift
      ;;
    -c|--cover)
      COVER=true
      shift
      ;;
    *)
      PACKAGES+=("$1")
      shift
      ;;
  esac
done

# Default to all packages if none specified
if [[ ${#PACKAGES[@]} -eq 0 ]]; then
  PACKAGES=("./...")
fi

# Build test flags
TEST_FLAGS=()

if [[ "${VERBOSE}" == "true" ]]; then
  TEST_FLAGS+=("-v")
fi

if [[ "${RACE}" == "true" ]]; then
  TEST_FLAGS+=("-race")
fi

if [[ "${UPDATE}" == "true" ]]; then
  TEST_FLAGS+=("-update")
fi

if [[ "${COVER}" == "true" ]]; then
  TEST_FLAGS+=("-cover")
fi

# Run the tests
echo "Running tests with flags: ${TEST_FLAGS[*]}"
echo "Packages: ${PACKAGES[*]}"

cd "${ROOT_DIR}"
go test "${TEST_FLAGS[@]}" "${PACKAGES[@]}"
