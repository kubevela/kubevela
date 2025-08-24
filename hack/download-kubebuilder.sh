#!/usr/bin/env bash
#!/usr/bin/env bash

# This script downloads and installs kubebuilder

set -e

# Constants
KUBEBUILDER_VERSION=${KUBEBUILDER_VERSION:-3.12.0}
INSTALL_DIR=${INSTALL_DIR:-/usr/local/kubebuilder}
BINARIES_DIR=${INSTALL_DIR}/bin
OS=$(uname | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
if [ "${ARCH}" == "x86_64" ]; then
  ARCH="amd64"
fi

echo "==> Installing kubebuilder version ${KUBEBUILDER_VERSION} to ${INSTALL_DIR}"
echo "OS: ${OS}, ARCH: ${ARCH}"

# Create install directory if it doesn't exist
if [ ! -d "${INSTALL_DIR}" ]; then
  echo "==> Creating directory ${INSTALL_DIR}"
  mkdir -p "${INSTALL_DIR}" || sudo mkdir -p "${INSTALL_DIR}"
fi

# Create bin directory if it doesn't exist
if [ ! -d "${BINARIES_DIR}" ]; then
  echo "==> Creating bin directory ${BINARIES_DIR}"
  mkdir -p "${BINARIES_DIR}" || sudo mkdir -p "${BINARIES_DIR}"
fi

# Ensure install directory is writable
if [ ! -w "${INSTALL_DIR}" ]; then
  echo "==> Making ${INSTALL_DIR} writable with sudo"
  sudo chmod -R 777 "${INSTALL_DIR}"
fi

# Ensure binaries directory is writable
if [ ! -w "${BINARIES_DIR}" ]; then
  echo "==> Making ${BINARIES_DIR} writable with sudo"
  sudo chmod -R 777 "${BINARIES_DIR}"
fi

# Download kubebuilder
echo "==> Downloading kubebuilder version ${KUBEBUILDER_VERSION}"
TMP_DIR=$(mktemp -d)
pushd "${TMP_DIR}" > /dev/null

# For Linux, download the binary directly
if [ "${OS}" == "linux" ]; then
  # URL for the kubebuilder binary
  KUBEBUILDER_URL="https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${KUBEBUILDER_VERSION}/kubebuilder_${OS}_${ARCH}"
  curl -L --retry 5 --retry-delay 3 "${KUBEBUILDER_URL}" -o kubebuilder
  chmod +x kubebuilder
  cp kubebuilder "${BINARIES_DIR}/kubebuilder" || sudo cp kubebuilder "${BINARIES_DIR}/kubebuilder"
else
  # For other platforms, download the archive
  KUBEBUILDER_URL="https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${KUBEBUILDER_VERSION}/kubebuilder_v${KUBEBUILDER_VERSION}_${OS}_${ARCH}.tar.gz"
  curl -L --retry 5 --retry-delay 3 "${KUBEBUILDER_URL}" -o kubebuilder.tar.gz
  tar -zxvf kubebuilder.tar.gz
  cp kubebuilder_v${KUBEBUILDER_VERSION}_${OS}_${ARCH}/bin/kubebuilder "${BINARIES_DIR}/kubebuilder" || sudo cp kubebuilder_v${KUBEBUILDER_VERSION}_${OS}_${ARCH}/bin/kubebuilder "${BINARIES_DIR}/kubebuilder"
fi

# Make binary executable (in case it's not already)
chmod +x "${BINARIES_DIR}/kubebuilder" || sudo chmod +x "${BINARIES_DIR}/kubebuilder"

popd > /dev/null
rm -rf "${TMP_DIR}"

echo "✓ kubebuilder installed successfully at ${BINARIES_DIR}/kubebuilder"

# Verify kubebuilder is accessible
echo "==> Verifying kubebuilder installation"
${BINARIES_DIR}/kubebuilder version || echo "WARNING: kubebuilder not working properly"
# This script downloads and installs kubebuilder

set -e

# Constants
KUBEBUILDER_VERSION=${KUBEBUILDER_VERSION:-3.12.0}
INSTALL_DIR=${INSTALL_DIR:-/usr/local/kubebuilder}
OS=$(uname | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
if [ "${ARCH}" == "x86_64" ]; then
  ARCH="amd64"
fi

echo "==> Installing kubebuilder version ${KUBEBUILDER_VERSION} to ${INSTALL_DIR}"
echo "OS: ${OS}, ARCH: ${ARCH}"

# Create install directory if it doesn't exist
if [ ! -d "${INSTALL_DIR}" ]; then
  echo "==> Creating directory ${INSTALL_DIR}"
  mkdir -p "${INSTALL_DIR}" || sudo mkdir -p "${INSTALL_DIR}"
fi

# Ensure install directory is writable
if [ ! -w "${INSTALL_DIR}" ]; then
  echo "==> Making ${INSTALL_DIR} writable with sudo"
  sudo chmod -R 777 "${INSTALL_DIR}"
fi

# Function to download a file with progress
download_with_progress() {
    local url=$1
    local output=$2
    echo "Downloading $url to $output"
    curl -L --progress-bar --retry 5 --retry-delay 3 "$url" -o "$output"
}

echo "==> Downloading kubebuilder version ${KUBEBUILDER_VERSION}"

TMP_DIR=$(mktemp -d)
pushd "${TMP_DIR}" > /dev/null

KUBEBUILDER_URL="https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${KUBEBUILDER_VERSION}/kubebuilder_${OS}_${ARCH}"
download_with_progress "${KUBEBUILDER_URL}" "kubebuilder"

# Create bin directory
mkdir -p "${INSTALL_DIR}/bin"

# Copy binary to the target directory
echo "==> Installing kubebuilder binary to ${INSTALL_DIR}/bin"
cp kubebuilder "${INSTALL_DIR}/bin/kubebuilder" || sudo cp kubebuilder "${INSTALL_DIR}/bin/kubebuilder"

# Make binary executable
chmod +x "${INSTALL_DIR}/bin/kubebuilder" || sudo chmod +x "${INSTALL_DIR}/bin/kubebuilder"

popd > /dev/null
rm -rf "${TMP_DIR}"

echo "✓ kubebuilder installed successfully"

# Set environment variables
echo "==> Setting environment variables"
export PATH="${INSTALL_DIR}/bin:${PATH}"
chmod +x hack/download-kubebuilder.sh
echo "==> kubebuilder installation complete"
echo "PATH=${PATH}"

# Verify binary is accessible
echo "==> Verifying kubebuilder is accessible"
${INSTALL_DIR}/bin/kubebuilder version || echo "WARNING: kubebuilder not working properly"
