#!/usr/bin/env bash

# This script sets up a local test environment for KubeVela

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

KUBEBUILDER_DIR=${KUBEBUILDER_DIR:-/usr/local/kubebuilder}
KUBEBUILDER_BIN=${KUBEBUILDER_BIN:-${KUBEBUILDER_DIR}/bin}

echo "===========> Setting up local test environment"

# Create kubebuilder directory if it doesn't exist
if [ ! -d "${KUBEBUILDER_BIN}" ]; then
  echo "Creating directory ${KUBEBUILDER_BIN}"
  mkdir -p "${KUBEBUILDER_BIN}" || sudo mkdir -p "${KUBEBUILDER_BIN}"
fi

# Ensure directory is writable
if [ ! -w "${KUBEBUILDER_BIN}" ]; then
  echo "Directory ${KUBEBUILDER_BIN} is not writable, attempting to fix permissions"
  sudo chmod -R 777 "${KUBEBUILDER_BIN}" || true
fi

# Download and install etcd if it doesn't exist
if [ ! -f "${KUBEBUILDER_BIN}/etcd" ]; then
  echo "===========> Downloading etcd"
  mkdir -p /tmp/etcd-download
  curl -L --retry 5 --retry-delay 3 https://github.com/etcd-io/etcd/releases/download/v3.5.7/etcd-v3.5.7-linux-amd64.tar.gz -o /tmp/etcd.tar.gz
  tar -xzf /tmp/etcd.tar.gz -C /tmp/etcd-download --strip-components=1
  cp /tmp/etcd-download/etcd "${KUBEBUILDER_BIN}/" || sudo cp /tmp/etcd-download/etcd "${KUBEBUILDER_BIN}/"
  cp /tmp/etcd-download/etcdctl "${KUBEBUILDER_BIN}/" || sudo cp /tmp/etcd-download/etcdctl "${KUBEBUILDER_BIN}/"
  chmod +x "${KUBEBUILDER_BIN}/etcd" || sudo chmod +x "${KUBEBUILDER_BIN}/etcd"
  chmod +x "${KUBEBUILDER_BIN}/etcdctl" || sudo chmod +x "${KUBEBUILDER_BIN}/etcdctl"
  rm -rf /tmp/etcd.tar.gz /tmp/etcd-download
  echo "===========> Etcd installed successfully"
else
  echo "===========> Etcd already installed"
fi

# Download and install kubebuilder if it doesn't exist
if [ ! -f "${KUBEBUILDER_BIN}/kubebuilder" ]; then
  echo "===========> Downloading kubebuilder"
  curl -L --retry 5 --retry-delay 3 https://github.com/kubernetes-sigs/kubebuilder/releases/download/v3.9.0/kubebuilder_linux_amd64 -o "${KUBEBUILDER_BIN}/kubebuilder" || \
    sudo curl -L --retry 5 --retry-delay 3 https://github.com/kubernetes-sigs/kubebuilder/releases/download/v3.9.0/kubebuilder_linux_amd64 -o "${KUBEBUILDER_BIN}/kubebuilder"
  chmod +x "${KUBEBUILDER_BIN}/kubebuilder" || sudo chmod +x "${KUBEBUILDER_BIN}/kubebuilder"
  echo "===========> Kubebuilder installed successfully"
else
  echo "===========> Kubebuilder already installed"
fi

# Verify installation
echo "===========> Verifying installation"
ls -la "${KUBEBUILDER_BIN}"

echo "===========> Local test environment setup complete"
echo "===========> You can run tests with: PATH=\"${KUBEBUILDER_BIN}:\$PATH\" KUBEBUILDER_ASSETS=\"${KUBEBUILDER_BIN}\" make test"
