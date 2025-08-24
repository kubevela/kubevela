#!/usr/bin/env bash
#!/usr/bin/env bash

# This script fixes permissions for the kubebuilder test binaries

set -e

KUBEBUILDER_DIR=${KUBEBUILDER_DIR:-/usr/local/kubebuilder}
BINARIES_DIR=${KUBEBUILDER_ASSETS:-${KUBEBUILDER_DIR}/bin}

echo "==> Fixing permissions for test binaries in ${BINARIES_DIR}"

# Check if directory exists
if [ ! -d "${BINARIES_DIR}" ]; then
  echo "Error: Directory ${BINARIES_DIR} does not exist"
  exit 1
fi

# List of binaries to check
BINARIES=(
  "etcd"
  "etcdctl"
  "kube-apiserver"
  "kubectl"
  "kubebuilder"
)

# Fix permissions for each binary
for binary in "${BINARIES[@]}"; do
  binary_path="${BINARIES_DIR}/${binary}"
  
  if [ -f "${binary_path}" ]; then
    echo "Fixing permissions for ${binary_path}"
    
    # Try with regular user permissions first
    chmod 755 "${binary_path}" 2>/dev/null || {
      echo "Using sudo to fix permissions for ${binary_path}"
      sudo chmod 755 "${binary_path}"
    }
    
    # Check ownership and fix if needed
    owner=$(stat -c '%U' "${binary_path}" 2>/dev/null || echo "unknown")
    if [ "${owner}" != "$(whoami)" ] && [ "${owner}" != "root" ]; then
      echo "Changing ownership of ${binary_path}"
      sudo chown root:root "${binary_path}"
    fi
    
    # Verify permissions
    ls -la "${binary_path}"
  else
    echo "Warning: Binary ${binary_path} not found"
  fi
done

# Fix directory permissions
echo "Fixing directory permissions"
chmod 755 "${BINARIES_DIR}" 2>/dev/null || sudo chmod 755 "${BINARIES_DIR}"

echo "==> Permissions fixed successfully"
# This script fixes permissions for kubebuilder binaries

set -o errexit
set -o nounset
set -o pipefail

# Determine script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Default kubebuilder bin directory
KUBEBUILDER_BIN_DIR="${KUBEBUILDER_ASSETS:-/usr/local/kubebuilder/bin}"
mkdir -p "${KUBEBUILDER_BIN_DIR}"

echo "Fixing permissions for kubebuilder binaries in ${KUBEBUILDER_BIN_DIR}"

# Fix directory permissions
sudo chmod 755 "${KUBEBUILDER_BIN_DIR}"
sudo chown -R "$(whoami)" "${KUBEBUILDER_BIN_DIR}"

# Fix binary permissions
BINARIES=("etcd" "kube-apiserver" "kubectl")
for binary in "${BINARIES[@]}"; do
  BINARY_PATH="${KUBEBUILDER_BIN_DIR}/${binary}"
  if [[ -f "${BINARY_PATH}" ]]; then
    echo "Setting executable permissions for ${binary}"
    chmod +x "${BINARY_PATH}"
    
    # Verify permissions
    if [[ -x "${BINARY_PATH}" ]]; then
      echo "✓ ${binary} is now executable"
    else
      echo "✗ Failed to make ${binary} executable"
      exit 1
    fi
  else
    echo "Binary ${binary} not found at ${BINARY_PATH}"
  fi
done

echo "Permission fix complete!"
