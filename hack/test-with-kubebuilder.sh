#!/usr/bin/env bash

# This script runs tests with kubebuilder binaries

set -e

# Check if kubebuilder binaries exist
KUBEBUILDER_ASSETS=${KUBEBUILDER_ASSETS:-/usr/local/kubebuilder/bin}
REQUIRED_BINARIES=("etcd" "kube-apiserver" "kubectl")
MISSING_BINARIES=()

for bin in "${REQUIRED_BINARIES[@]}"; do
  if [ ! -f "${KUBEBUILDER_ASSETS}/${bin}" ]; then
    MISSING_BINARIES+=("${bin}")
  fi
done

# Install kubebuilder if binaries are missing
if [ ${#MISSING_BINARIES[@]} -gt 0 ]; then
  echo "Missing kubebuilder binaries: ${MISSING_BINARIES[*]}"
  echo "Installing kubebuilder..."
  ./hack/setup-kubebuilder.sh
  
  # Check again after installation
  MISSING_BINARIES=()
  for bin in "${REQUIRED_BINARIES[@]}"; do
    if [ ! -f "${KUBEBUILDER_ASSETS}/${bin}" ]; then
      MISSING_BINARIES+=("${bin}")
    fi
  done
  
  if [ ${#MISSING_BINARIES[@]} -gt 0 ]; then
    echo "Failed to install kubebuilder binaries: ${MISSING_BINARIES[*]}"
    exit 1
  fi
fi

# Set environment variable for tests
export KUBEBUILDER_ASSETS=${KUBEBUILDER_ASSETS}

# Run tests with proper output
echo "Running tests with KUBEBUILDER_ASSETS=${KUBEBUILDER_ASSETS}"
go test "$@"
