#!/usr/bin/env bash

# This script downloads and installs kubebuilder test binaries

set -e

# Constants
KUBEBUILDER_VERSION=${KUBEBUILDER_VERSION:-2.3.2}
INSTALL_DIR=${INSTALL_DIR:-/usr/local/kubebuilder}
OS=$(uname | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
if [ "${ARCH}" == "x86_64" ]; then
  ARCH="amd64"
fi

echo "==> Installing kubebuilder v${KUBEBUILDER_VERSION} to ${INSTALL_DIR}"
echo "OS: ${OS}, ARCH: ${ARCH}"

# Create temp directory
TMP_DIR=$(mktemp -d)
cd "${TMP_DIR}"

# Download kubebuilder
DOWNLOAD_URL="https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${KUBEBUILDER_VERSION}/kubebuilder_${KUBEBUILDER_VERSION}_${OS}_${ARCH}.tar.gz"
echo "==> Downloading kubebuilder from ${DOWNLOAD_URL}"
curl -sL "${DOWNLOAD_URL}" | tar -xz

# Create install directory if it doesn't exist
if [ ! -d "${INSTALL_DIR}" ]; then
  echo "==> Creating directory ${INSTALL_DIR}"
  mkdir -p "${INSTALL_DIR}" || sudo mkdir -p "${INSTALL_DIR}"
fi

# Copy binaries to install directory
echo "==> Copying binaries to ${INSTALL_DIR}"
if [ -w "${INSTALL_DIR}" ]; then
  cp -r kubebuilder_${KUBEBUILDER_VERSION}_${OS}_${ARCH}/* "${INSTALL_DIR}/"
else
  sudo cp -r kubebuilder_${KUBEBUILDER_VERSION}_${OS}_${ARCH}/* "${INSTALL_DIR}/"
fi

# Make binaries executable
if [ -w "${INSTALL_DIR}/bin" ]; then
  chmod +x ${INSTALL_DIR}/bin/*
else
  sudo chmod +x ${INSTALL_DIR}/bin/*
fi

# Clean up temp directory
cd -
rm -rf "${TMP_DIR}"

echo "==> Verifying installation"
${INSTALL_DIR}/bin/etcd --version || echo "etcd verification failed"
${INSTALL_DIR}/bin/kube-apiserver --version || echo "kube-apiserver verification failed"
${INSTALL_DIR}/bin/kubectl version --client || echo "kubectl verification failed"

echo "==> Installation complete!"
echo "Add ${INSTALL_DIR}/bin to your PATH to use the installed binaries:"
echo "export PATH=\$PATH:${INSTALL_DIR}/bin"
