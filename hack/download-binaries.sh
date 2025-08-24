#!/usr/bin/env bash
#!/usr/bin/env bash
#!/usr/bin/env bash

# This script downloads the kubebuilder binaries required for testing

set -o errexit
set -o nounset
set -o pipefail

# Determine script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Set target directory and create if it doesn't exist
KUBEBUILDER_BIN_DIR="${KUBEBUILDER_BIN_DIR:-/usr/local/kubebuilder/bin}"
mkdir -p "${KUBEBUILDER_BIN_DIR}"

# Detect platform
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case ${ARCH} in
  x86_64)
    ARCH=amd64
    ;;
  aarch64)
    ARCH=arm64
    ;;
  *)
    echo "Unsupported architecture: ${ARCH}"
    exit 1
    ;;
esac

# Define versions
ETCD_VERSION="${ETCD_VERSION:-v3.5.7}"
KUBE_VERSION="${KUBE_VERSION:-v1.26.1}"

# Etcd download URLs
ETCD_BASE_URL="https://github.com/etcd-io/etcd/releases/download"
ETCD_URL="${ETCD_BASE_URL}/${ETCD_VERSION}/etcd-${ETCD_VERSION}-${OS}-${ARCH}.tar.gz"

# Kubernetes download URLs
K8S_BASE_URL="https://dl.k8s.io/release"
K8S_API_URL="${K8S_BASE_URL}/${KUBE_VERSION}/bin/${OS}/${ARCH}/kube-apiserver"
KUBECTL_URL="${K8S_BASE_URL}/${KUBE_VERSION}/bin/${OS}/${ARCH}/kubectl"

download_file() {
  local url=$1
  local output=$2
  echo "Downloading from ${url} to ${output}"
  
  # Try curl first, then wget if curl fails
  if command -v curl &> /dev/null; then
    if ! curl -sSL --retry 5 --retry-delay 3 "${url}" -o "${output}"; then
      echo "curl download failed, trying wget..."
      if command -v wget &> /dev/null; then
        wget -q --tries=5 --waitretry=3 "${url}" -O "${output}"
      else
        echo "Both curl and wget failed. Please install either curl or wget."
        return 1
      fi
    fi
  elif command -v wget &> /dev/null; then
    wget -q --tries=5 --waitretry=3 "${url}" -O "${output}"
  else
    echo "Neither curl nor wget is available. Please install either curl or wget."
    return 1
  fi
}

# Create temporary directory
TMP_DIR=$(mktemp -d)
trap 'rm -rf ${TMP_DIR}' EXIT

# Download and install etcd
echo "===========> Downloading etcd ${ETCD_VERSION}"
ETCD_TMP_FILE="${TMP_DIR}/etcd.tar.gz"
download_file "${ETCD_URL}" "${ETCD_TMP_FILE}"

echo "Extracting etcd..."
tar -xzf "${ETCD_TMP_FILE}" -C "${TMP_DIR}"
ETCD_DIR=$(find "${TMP_DIR}" -name "etcd-*" -type d | head -n 1)
if [[ -d "${ETCD_DIR}" ]]; then
  cp "${ETCD_DIR}/etcd" "${KUBEBUILDER_BIN_DIR}/"
  cp "${ETCD_DIR}/etcdctl" "${KUBEBUILDER_BIN_DIR}/" || true
  chmod +x "${KUBEBUILDER_BIN_DIR}/etcd" "${KUBEBUILDER_BIN_DIR}/etcdctl" || true
  echo "===========> Etcd installed successfully at ${KUBEBUILDER_BIN_DIR}/etcd"
else
  echo "Failed to extract etcd. Extraction directory not found."
  exit 1
fi

# Download and install kube-apiserver
echo "===========> Downloading kube-apiserver ${KUBE_VERSION}"
download_file "${K8S_API_URL}" "${KUBEBUILDER_BIN_DIR}/kube-apiserver"
chmod +x "${KUBEBUILDER_BIN_DIR}/kube-apiserver"
echo "===========> Kube-apiserver installed successfully at ${KUBEBUILDER_BIN_DIR}/kube-apiserver"

# Download and install kubectl
echo "===========> Downloading kubectl ${KUBE_VERSION}"
download_file "${KUBECTL_URL}" "${KUBEBUILDER_BIN_DIR}/kubectl"
chmod +x "${KUBEBUILDER_BIN_DIR}/kubectl"
echo "===========> Kubectl installed successfully at ${KUBEBUILDER_BIN_DIR}/kubectl"

# Verify installed binaries
echo "Verifying installed binaries..."
for binary in etcd kube-apiserver kubectl; do
  if [[ -x "${KUBEBUILDER_BIN_DIR}/${binary}" ]]; then
    echo "✓ ${binary} installed successfully"
  else
    echo "✗ ${binary} installation failed"
    exit 1
  fi
done

# Set KUBEBUILDER_ASSETS environment variable if not already set
if [[ -z "${KUBEBUILDER_ASSETS:-}" ]]; then
  echo "export KUBEBUILDER_ASSETS=${KUBEBUILDER_BIN_DIR}" >> "${ROOT_DIR}/.envrc"
  echo "Added KUBEBUILDER_ASSETS to .envrc file"
  echo ""
  echo "To use immediately, run:"
  echo "export KUBEBUILDER_ASSETS=${KUBEBUILDER_BIN_DIR}"
fi

echo "All required binaries have been installed successfully!"
# This script downloads all the required binaries for running KubeVela tests
# It handles the installation of etcd, kube-apiserver, and kubectl

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

KUBEBUILDER_DIR=${KUBEBUILDER_DIR:-/usr/local/kubebuilder}
KUBEBUILDER_BIN=${KUBEBUILDER_BIN:-${KUBEBUILDER_DIR}/bin}

ETCD_VERSION=${ETCD_VERSION:-3.5.7}
KUBE_APISERVER_VERSION=${KUBE_APISERVER_VERSION:-1.26.1}
KUBECTL_VERSION=${KUBECTL_VERSION:-1.26.1}

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

# Download and install kubectl
echo "===========> Downloading kubectl v${KUBECTL_VERSION}"
curl -L --retry 5 --retry-delay 3 https://dl.k8s.io/v${KUBECTL_VERSION}/bin/linux/amd64/kubectl -o "${KUBEBUILDER_BIN}/kubectl" 2>/dev/null || \
  sudo curl -L --retry 5 --retry-delay 3 https://dl.k8s.io/v${KUBECTL_VERSION}/bin/linux/amd64/kubectl -o "${KUBEBUILDER_BIN}/kubectl"
chmod +x "${KUBEBUILDER_BIN}/kubectl" 2>/dev/null || sudo chmod +x "${KUBEBUILDER_BIN}/kubectl"
echo "===========> kubectl installed successfully at ${KUBEBUILDER_BIN}/kubectl"

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

if [ -f "${KUBEBUILDER_BIN}/kubectl" ] && [ -x "${KUBEBUILDER_BIN}/kubectl" ]; then
  echo "✓ kubectl is installed and executable"
  "${KUBEBUILDER_BIN}/kubectl" version --client || echo "WARNING: kubectl version check failed"
else
  echo "✗ kubectl is not properly installed"
  exit 1
fi

echo "===========> Installation complete"
echo "You can now run tests with:"
echo "export PATH=\"${KUBEBUILDER_BIN}:\$PATH\""
echo "export KUBEBUILDER_ASSETS=\"${KUBEBUILDER_BIN}\""
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
