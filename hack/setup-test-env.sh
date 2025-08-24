#!/usr/bin/env bash
#!/usr/bin/env bash
#!/usr/bin/env bash

# This script sets up the test environment with proper permissions

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Set up kubebuilder
echo "==> Setting up kubebuilder"
"${SCRIPT_DIR}/setup-kubebuilder.sh"

# Download test binaries
echo "==> Downloading test binaries"
"${SCRIPT_DIR}/download-test-binaries.sh"

# Fix permissions
echo "==> Fixing permissions"
"${SCRIPT_DIR}/fix-permissions.sh"

# Set environment variables
export KUBEBUILDER_ASSETS="/usr/local/kubebuilder/bin"
echo "==> Set KUBEBUILDER_ASSETS=${KUBEBUILDER_ASSETS}"

# Verify setup
echo "==> Verifying test environment setup"
for binary in etcd etcdctl kube-apiserver kubectl; do
  if [ -x "${KUBEBUILDER_ASSETS}/${binary}" ]; then
    echo "✓ ${binary} is properly installed"
  else
    echo "✗ ${binary} is missing or has incorrect permissions"
    exit 1
  fi
done

echo "==> Test environment is ready"
echo "You can run tests with: KUBEBUILDER_ASSETS=${KUBEBUILDER_ASSETS} go test ./..."
# This script sets up a complete test environment for KubeVela
# It downloads and installs all required binaries for running the tests

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

KUBEBUILDER_VERSION=${KUBEBUILDER_VERSION:-3.9.0}
ETCD_VERSION=${ETCD_VERSION:-3.5.7}
KUBE_APISERVER_VERSION=${KUBE_APISERVER_VERSION:-1.26.1}
KUBECTL_VERSION=${KUBECTL_VERSION:-1.26.1}

KUBEBUILDER_DIR=${KUBEBUILDER_DIR:-/usr/local/kubebuilder}
KUBEBUILDER_BIN=${KUBEBUILDER_BIN:-${KUBEBUILDER_DIR}/bin}
OS=$(uname | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
if [ "${ARCH}" == "x86_64" ]; then
  ARCH="amd64"
fi

echo "===========> Setting up test environment"
echo "OS: ${OS}, ARCH: ${ARCH}"
echo "KUBEBUILDER_DIR: ${KUBEBUILDER_DIR}"
echo "KUBEBUILDER_BIN: ${KUBEBUILDER_BIN}"

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
function download_etcd() {
  echo "===========> Downloading etcd v${ETCD_VERSION}"
  mkdir -p /tmp/etcd-download
  curl -L --retry 5 --retry-delay 3 https://github.com/etcd-io/etcd/releases/download/v${ETCD_VERSION}/etcd-v${ETCD_VERSION}-linux-amd64.tar.gz -o /tmp/etcd.tar.gz
  tar -xzf /tmp/etcd.tar.gz -C /tmp/etcd-download --strip-components=1
  
  cp /tmp/etcd-download/etcd "${KUBEBUILDER_BIN}/" || sudo cp /tmp/etcd-download/etcd "${KUBEBUILDER_BIN}/"
  cp /tmp/etcd-download/etcdctl "${KUBEBUILDER_BIN}/" || sudo cp /tmp/etcd-download/etcdctl "${KUBEBUILDER_BIN}/"
  chmod +x "${KUBEBUILDER_BIN}/etcd" || sudo chmod +x "${KUBEBUILDER_BIN}/etcd"
  chmod +x "${KUBEBUILDER_BIN}/etcdctl" || sudo chmod +x "${KUBEBUILDER_BIN}/etcdctl"
  rm -rf /tmp/etcd.tar.gz /tmp/etcd-download
  echo "===========> Etcd installed successfully at ${KUBEBUILDER_BIN}/etcd"
}

# Download and install kube-apiserver
function download_kube_apiserver() {
  echo "===========> Downloading kube-apiserver v${KUBE_APISERVER_VERSION}"
  curl -L --retry 5 --retry-delay 3 https://dl.k8s.io/v${KUBE_APISERVER_VERSION}/bin/linux/amd64/kube-apiserver -o "${KUBEBUILDER_BIN}/kube-apiserver" || \
    sudo curl -L --retry 5 --retry-delay 3 https://dl.k8s.io/v${KUBE_APISERVER_VERSION}/bin/linux/amd64/kube-apiserver -o "${KUBEBUILDER_BIN}/kube-apiserver"
  chmod +x "${KUBEBUILDER_BIN}/kube-apiserver" || sudo chmod +x "${KUBEBUILDER_BIN}/kube-apiserver"
  echo "===========> kube-apiserver installed successfully at ${KUBEBUILDER_BIN}/kube-apiserver"
}
#!/usr/bin/env bash
#!/usr/bin/env bash

# This script sets up the environment for running tests

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Download test binaries if they don't exist
echo "==> Checking test binaries"
"${ROOT_DIR}/hack/download-test-binaries.sh"

# Set environment variables
export KUBEBUILDER_ASSETS=${KUBEBUILDER_ASSETS:-/usr/local/kubebuilder/bin}
export PATH="${KUBEBUILDER_ASSETS}:${PATH}"

echo "==> Test environment setup complete"
echo "KUBEBUILDER_ASSETS=${KUBEBUILDER_ASSETS}"
echo "PATH=${PATH}"

# Run the command passed to this script
if [ $# -gt 0 ]; then
    echo "==> Running command: $@"
    exec "$@"
fi
# This script sets up the test environment for running tests

set -e

KUBEBUILDER_DIR=${KUBEBUILDER_DIR:-/usr/local/kubebuilder}
KUBEBUILDER_BIN=${KUBEBUILDER_BIN:-${KUBEBUILDER_DIR}/bin}
ETCD_VERSION=${ETCD_VERSION:-3.5.7}
KUBE_APISERVER_VERSION=${KUBE_APISERVER_VERSION:-1.26.1}

# Create directories if they don't exist
mkdir -p ${KUBEBUILDER_BIN} || sudo mkdir -p ${KUBEBUILDER_BIN}

# Make directory writable
chmod -R 777 ${KUBEBUILDER_BIN} 2>/dev/null || sudo chmod -R 777 ${KUBEBUILDER_BIN}

# Function to download etcd
download_etcd() {
  echo "Downloading etcd version ${ETCD_VERSION}"
  
  TMP_DIR=$(mktemp -d)
  pushd ${TMP_DIR}
  
  curl -L --retry 5 --retry-delay 3 https://github.com/etcd-io/etcd/releases/download/v${ETCD_VERSION}/etcd-v${ETCD_VERSION}-linux-amd64.tar.gz -o etcd.tar.gz
  tar -xzf etcd.tar.gz --strip-components=1
  
  cp etcd ${KUBEBUILDER_BIN}/etcd 2>/dev/null || sudo cp etcd ${KUBEBUILDER_BIN}/etcd
  cp etcdctl ${KUBEBUILDER_BIN}/etcdctl 2>/dev/null || sudo cp etcdctl ${KUBEBUILDER_BIN}/etcdctl
  
  chmod +x ${KUBEBUILDER_BIN}/etcd 2>/dev/null || sudo chmod +x ${KUBEBUILDER_BIN}/etcd
  chmod +x ${KUBEBUILDER_BIN}/etcdctl 2>/dev/null || sudo chmod +x ${KUBEBUILDER_BIN}/etcdctl
  
  popd
  rm -rf ${TMP_DIR}
  
  echo "etcd installation complete"
}

# Function to download kube-apiserver
download_kube_apiserver() {
  echo "Downloading kube-apiserver version ${KUBE_APISERVER_VERSION}"
  
  curl -L --retry 5 --retry-delay 3 https://dl.k8s.io/v${KUBE_APISERVER_VERSION}/bin/linux/amd64/kube-apiserver -o ${KUBEBUILDER_BIN}/kube-apiserver 2>/dev/null || \
    sudo curl -L --retry 5 --retry-delay 3 https://dl.k8s.io/v${KUBE_APISERVER_VERSION}/bin/linux/amd64/kube-apiserver -o ${KUBEBUILDER_BIN}/kube-apiserver
  
  chmod +x ${KUBEBUILDER_BIN}/kube-apiserver 2>/dev/null || sudo chmod +x ${KUBEBUILDER_BIN}/kube-apiserver
  
  echo "kube-apiserver installation complete"
}

# Check if etcd is available
if [ ! -f "${KUBEBUILDER_BIN}/etcd" ]; then
  echo "etcd not found, downloading..."
  download_etcd
else
  echo "etcd already installed at ${KUBEBUILDER_BIN}/etcd"
fi

# Check if kube-apiserver is available
if [ ! -f "${KUBEBUILDER_BIN}/kube-apiserver" ]; then
  echo "kube-apiserver not found, downloading..."
  download_kube_apiserver
else
  echo "kube-apiserver already installed at ${KUBEBUILDER_BIN}/kube-apiserver"
fi

# Verify binaries are installed and executable
echo "Verifying test binaries..."
if [ -x "${KUBEBUILDER_BIN}/etcd" ] && [ -x "${KUBEBUILDER_BIN}/kube-apiserver" ]; then
  echo "All test binaries are installed and executable."
  echo "Test environment setup complete"
  
  # Set environment variables
  export PATH="${KUBEBUILDER_BIN}:${PATH}"
  export KUBEBUILDER_ASSETS="${KUBEBUILDER_BIN}"
  
  echo "Environment variables set:"
  echo "PATH=${PATH}"
  echo "KUBEBUILDER_ASSETS=${KUBEBUILDER_ASSETS}"
else
  echo "ERROR: Some binaries are missing or not executable"
  ls -la ${KUBEBUILDER_BIN}
  exit 1
fi
# Download and install kubectl
function download_kubectl() {
  echo "===========> Downloading kubectl v${KUBECTL_VERSION}"
  curl -L --retry 5 --retry-delay 3 https://dl.k8s.io/v${KUBECTL_VERSION}/bin/linux/amd64/kubectl -o "${KUBEBUILDER_BIN}/kubectl" || \
    sudo curl -L --retry 5 --retry-delay 3 https://dl.k8s.io/v${KUBECTL_VERSION}/bin/linux/amd64/kubectl -o "${KUBEBUILDER_BIN}/kubectl"
  chmod +x "${KUBEBUILDER_BIN}/kubectl" || sudo chmod +x "${KUBEBUILDER_BIN}/kubectl"
  echo "===========> kubectl installed successfully at ${KUBEBUILDER_BIN}/kubectl"
}

# Download and install kubebuilder
function download_kubebuilder() {
  echo "===========> Downloading kubebuilder v${KUBEBUILDER_VERSION}"
  curl -L --retry 5 --retry-delay 3 https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${KUBEBUILDER_VERSION}/kubebuilder_linux_amd64 -o "${KUBEBUILDER_BIN}/kubebuilder" || \
    sudo curl -L --retry 5 --retry-delay 3 https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${KUBEBUILDER_VERSION}/kubebuilder_linux_amd64 -o "${KUBEBUILDER_BIN}/kubebuilder"
  chmod +x "${KUBEBUILDER_BIN}/kubebuilder" || sudo chmod +x "${KUBEBUILDER_BIN}/kubebuilder"
  echo "===========> kubebuilder installed successfully at ${KUBEBUILDER_BIN}/kubebuilder"
}

# Check for binaries and download if missing
if [ ! -f "${KUBEBUILDER_BIN}/etcd" ] || [ ! -x "${KUBEBUILDER_BIN}/etcd" ]; then
  download_etcd
else
  echo "===========> etcd already installed at ${KUBEBUILDER_BIN}/etcd"
fi

if [ ! -f "${KUBEBUILDER_BIN}/kube-apiserver" ] || [ ! -x "${KUBEBUILDER_BIN}/kube-apiserver" ]; then
  download_kube_apiserver
else
  echo "===========> kube-apiserver already installed at ${KUBEBUILDER_BIN}/kube-apiserver"
fi

if [ ! -f "${KUBEBUILDER_BIN}/kubectl" ] || [ ! -x "${KUBEBUILDER_BIN}/kubectl" ]; then
  download_kubectl
else
  echo "===========> kubectl already installed at ${KUBEBUILDER_BIN}/kubectl"
fi

if [ ! -f "${KUBEBUILDER_BIN}/kubebuilder" ] || [ ! -x "${KUBEBUILDER_BIN}/kubebuilder" ]; then
  download_kubebuilder
else
  echo "===========> kubebuilder already installed at ${KUBEBUILDER_BIN}/kubebuilder"
fi

# Verify installation
echo "===========> Verifying installation"
ls -la "${KUBEBUILDER_BIN}"

if [ -f "${KUBEBUILDER_BIN}/etcd" ] && [ -x "${KUBEBUILDER_BIN}/etcd" ]; then
  echo "✓ etcd is installed and executable"
  "${KUBEBUILDER_BIN}/etcd" --version || echo "WARNING: etcd version check failed"
else
  echo "✗ etcd is not properly installed"
fi

if [ -f "${KUBEBUILDER_BIN}/kube-apiserver" ] && [ -x "${KUBEBUILDER_BIN}/kube-apiserver" ]; then
  echo "✓ kube-apiserver is installed and executable"
  "${KUBEBUILDER_BIN}/kube-apiserver" --version || echo "WARNING: kube-apiserver version check failed"
else
  echo "✗ kube-apiserver is not properly installed"
fi

if [ -f "${KUBEBUILDER_BIN}/kubectl" ] && [ -x "${KUBEBUILDER_BIN}/kubectl" ]; then
  echo "✓ kubectl is installed and executable"
  "${KUBEBUILDER_BIN}/kubectl" version --client || echo "WARNING: kubectl version check failed"
else
  echo "✗ kubectl is not properly installed"
fi

echo "===========> Test environment setup complete"
echo "===========> To use this environment for tests, run:"
echo "export PATH=\"${KUBEBUILDER_BIN}:\$PATH\""
echo "export KUBEBUILDER_ASSETS=\"${KUBEBUILDER_BIN}\""
echo "===========> Or run tests with: KUBEBUILDER_ASSETS=\"${KUBEBUILDER_BIN}\" go test ./..."

# Export environment variables for the current session
export PATH="${KUBEBUILDER_BIN}:$PATH"
export KUBEBUILDER_ASSETS="${KUBEBUILDER_BIN}"
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
