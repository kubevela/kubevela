#!/usr/bin/env bash

# This script downloads and sets up the necessary binaries for running KubeVela tests

set -e

# Determine the architecture
ARCH=$(uname -m)
case $ARCH in
  x86_64)
    ARCH="amd64"
    ;;
  aarch64)
    ARCH="arm64"
    ;;
  *)
    echo "Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

# Determine the OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case $OS in
  linux)
    ;;
  darwin)
    ;;
  *)
    echo "Unsupported OS: $OS"
    exit 1
    ;;
esac

# Set Kubernetes version to download
K8S_VERSION=${ENVTEST_K8S_VERSION:-1.29.0}

# Create required directories
mkdir -p bin
mkdir -p /usr/local/kubebuilder/bin

# Download setup-envtest
if [ ! -f bin/setup-envtest ]; then
  echo "Downloading setup-envtest..."
  go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
  cp $(go env GOPATH)/bin/setup-envtest bin/
fi

# Download and set up the test environment
echo "Setting up test environment with Kubernetes ${K8S_VERSION}..."
KUBEBUILDER_ASSETS=$(bin/setup-envtest use ${K8S_VERSION} -p path)

# Copy assets to the expected location
echo "Copying test binaries to /usr/local/kubebuilder/bin..."
cp ${KUBEBUILDER_ASSETS}/* /usr/local/kubebuilder/bin/

echo "Test environment setup complete. Binaries installed in /usr/local/kubebuilder/bin"
echo "You can run your tests now."
