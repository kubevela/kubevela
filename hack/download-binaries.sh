#!/usr/bin/env bash

# This script directly downloads all the required binaries for testing
# It's a more reliable approach than depending on envtest setup

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

KUBEBUILDER_DIR=${KUBEBUILDER_DIR:-/usr/local/kubebuilder}
KUBEBUILDER_BIN=${KUBEBUILDER_BIN:-${KUBEBUILDER_DIR}/bin}

ETCD_VERSION=${ETCD_VERSION:-3.5.7}
KUBE_APISERVER_VERSION=${KUBE_APISERVER_VERSION:-1.26.1}

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

# Download and install etcd
echo "===========> Downloading etcd v${ETCD_VERSION}"
mkdir -p /tmp/etcd-download
curl -L --retry 5 --retry-delay 3 https://github.com/etcd-io/etcd/releases/download/v${ETCD_VERSION}/etcd-v${ETCD_VERSION}-linux-amd64.tar.gz -o /tmp/etcd.tar.gz
tar -xzf /tmp/etcd.tar.gz -C /tmp/etcd-download --strip-components=1

cp /tmp/etcd-download/etcd "${KUBEBUILDER_BIN}/etcd" 2>/dev/null || sudo cp /tmp/etcd-download/etcd "${KUBEBUILDER_BIN}/etcd"
cp /tmp/etcd-download/etcdctl "${KUBEBUILDER_BIN}/etcdctl" 2>/dev/null || sudo cp /tmp/etcd-download/etcdctl "${KUBEBUILDER_BIN}/etcdctl"
chmod +x "${KUBEBUILDER_BIN}/etcd" 2>/dev/null || sudo chmod +x "${KUBEBUILDER_BIN}/etcd"
chmod +x "${KUBEBUILDER_BIN}/etcdctl" 2>/dev/null || sudo chmod +x "${KUBEBUILDER_BIN}/etcdctl"
rm -rf /tmp/etcd.tar.gz /tmp/etcd-download
echo "===========> Etcd installed successfully at ${KUBEBUILDER_BIN}/etcd"

# Download and install kube-apiserver
echo "===========> Downloading kube-apiserver v${KUBE_APISERVER_VERSION}"
curl -L --retry 5 --retry-delay 3 https://dl.k8s.io/v${KUBE_APISERVER_VERSION}/bin/linux/amd64/kube-apiserver -o "${KUBEBUILDER_BIN}/kube-apiserver" 2>/dev/null || \
  sudo curl -L --retry 5 --retry-delay 3 https://dl.k8s.io/v${KUBE_APISERVER_VERSION}/bin/linux/amd64/kube-apiserver -o "${KUBEBUILDER_BIN}/kube-apiserver"
chmod +x "${KUBEBUILDER_BIN}/kube-apiserver" 2>/dev/null || sudo chmod +x "${KUBEBUILDER_BIN}/kube-apiserver"
echo "===========> kube-apiserver installed successfully at ${KUBEBUILDER_BIN}/kube-apiserver"

# Verify installation
echo "===========> Verifying installation"
ls -la "${KUBEBUILDER_BIN}"

if [ -f "${KUBEBUILDER_BIN}/etcd" ] && [ -x "${KUBEBUILDER_BIN}/etcd" ]; then
  echo "✓ etcd is installed and executable"
  "${KUBEBUILDER_BIN}/etcd" --version || echo "WARNING: etcd version check failed"
else
  echo "✗ etcd is not properly installed"
  exit 1
fi

if [ -f "${KUBEBUILDER_BIN}/kube-apiserver" ] && [ -x "${KUBEBUILDER_BIN}/kube-apiserver" ]; then
  echo "✓ kube-apiserver is installed and executable"
  "${KUBEBUILDER_BIN}/kube-apiserver" --version || echo "WARNING: kube-apiserver version check failed"
else
  echo "✗ kube-apiserver is not properly installed"
  exit 1
fi

echo "===========> Installation complete"
echo "You can now run tests with:"
echo "export PATH=\"${KUBEBUILDER_BIN}:\$PATH\""
echo "export KUBEBUILDER_ASSETS=\"${KUBEBUILDER_BIN}\""
