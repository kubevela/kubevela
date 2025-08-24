#!/usr/bin/env bash

# This script downloads and installs test binaries needed for controller-runtime's envtest

set -eo pipefail

# Constants
INSTALL_DIR=${INSTALL_DIR:-/usr/local/kubebuilder}
BINARIES_DIR=${KUBEBUILDER_ASSETS:-${INSTALL_DIR}/bin}
K8S_VERSION=${K8S_VERSION:-1.26.1}
ETCD_VERSION=${ETCD_VERSION:-3.5.7}
OS=$(uname | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
if [ "${ARCH}" == "x86_64" ]; then
  ARCH="amd64"
fi

echo "==> Installing test binaries to ${BINARIES_DIR}"
echo "OS: ${OS}, ARCH: ${ARCH}"
echo "Kubernetes version: ${K8S_VERSION}, etcd version: ${ETCD_VERSION}"

# Create install directory if it doesn't exist
if [ ! -d "${BINARIES_DIR}" ]; then
  echo "==> Creating directory ${BINARIES_DIR}"
  mkdir -p "${BINARIES_DIR}" || sudo mkdir -p "${BINARIES_DIR}"
fi

# Ensure binaries directory is writable
if [ ! -w "${BINARIES_DIR}" ]; then
  echo "==> Making ${BINARIES_DIR} writable with sudo"
  sudo chmod -R 777 "${BINARIES_DIR}"
fi

# Function to download a file with progress
download_with_progress() {
    local url=$1
    local output=$2
    echo "Downloading $url to $output"
    curl -L --progress-bar --retry 5 --retry-delay 3 "$url" -o "$output" || {
        echo "ERROR: Failed to download $url"
        return 1
    }
    return 0
}

# Function to download kubebuilder
download_kubebuilder() {
    echo "==> Downloading kubebuilder binary"
    
    local KUBEBUILDER_VERSION=${KUBEBUILDER_VERSION:-3.4.0}
    local TMP_DIR=$(mktemp -d)
    pushd "${TMP_DIR}" > /dev/null
    
    local KUBEBUILDER_URL="https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${KUBEBUILDER_VERSION}/kubebuilder_${KUBEBUILDER_VERSION}_${OS}_${ARCH}.tar.gz"
    echo "==> Downloading kubebuilder from ${KUBEBUILDER_URL}"
    
    if ! download_with_progress "${KUBEBUILDER_URL}" "kubebuilder.tar.gz"; then
        echo "ERROR: Failed to download kubebuilder"
        popd > /dev/null
        rm -rf "${TMP_DIR}"
        return 1
    fi
    
    echo "==> Extracting kubebuilder binary"
    tar -xzf kubebuilder.tar.gz --strip-components=1
    
    # Copy binary to the target directory
    echo "==> Installing kubebuilder binary to ${BINARIES_DIR}"
    cp bin/kubebuilder "${BINARIES_DIR}/kubebuilder" || sudo cp bin/kubebuilder "${BINARIES_DIR}/kubebuilder"
    
    # Make binary executable
    chmod +x "${BINARIES_DIR}/kubebuilder" || sudo chmod +x "${BINARIES_DIR}/kubebuilder"
    
    popd > /dev/null
    rm -rf "${TMP_DIR}"
    
    echo "✓ kubebuilder binary installed successfully"
    return 0
}

# Function to download etcd
download_etcd() {
    echo "==> Downloading etcd version ${ETCD_VERSION}"
    
    local TMP_DIR=$(mktemp -d)
    pushd "${TMP_DIR}" > /dev/null
    
    local ETCD_URL="https://github.com/etcd-io/etcd/releases/download/v${ETCD_VERSION}/etcd-v${ETCD_VERSION}-linux-amd64.tar.gz"
    if ! download_with_progress "${ETCD_URL}" "etcd.tar.gz"; then
        echo "ERROR: Failed to download etcd"
        popd > /dev/null
        rm -rf "${TMP_DIR}"
        return 1
    fi
    
    echo "==> Extracting etcd binaries"
    tar -xzf etcd.tar.gz --strip-components=1
    
    # Copy binaries to the target directory
    echo "==> Installing etcd binaries to ${BINARIES_DIR}"
    cp etcd "${BINARIES_DIR}/etcd" || sudo cp etcd "${BINARIES_DIR}/etcd"
    cp etcdctl "${BINARIES_DIR}/etcdctl" || sudo cp etcdctl "${BINARIES_DIR}/etcdctl"
    
    # Make binaries executable with proper permissions (755)
    chmod 755 "${BINARIES_DIR}/etcd" || sudo chmod 755 "${BINARIES_DIR}/etcd"
    chmod 755 "${BINARIES_DIR}/etcdctl" || sudo chmod 755 "${BINARIES_DIR}/etcdctl"
    
    popd > /dev/null
    rm -rf "${TMP_DIR}"
    
    echo "✓ etcd binaries installed successfully"
    return 0
}

# Function to download kube-apiserver
download_kube_apiserver() {
    echo "==> Downloading kube-apiserver version ${K8S_VERSION}"
    
    local API_SERVER_URL="https://dl.k8s.io/v${K8S_VERSION}/bin/linux/amd64/kube-apiserver"
    if ! download_with_progress "${API_SERVER_URL}" "${BINARIES_DIR}/kube-apiserver"; then
        echo "Trying with sudo..."
        if ! sudo curl -L --retry 5 --retry-delay 3 "${API_SERVER_URL}" -o "${BINARIES_DIR}/kube-apiserver"; then
            echo "ERROR: Failed to download kube-apiserver"
            return 1
        fi
    fi
    
    # Make binary executable with proper permissions (755)
    chmod 755 "${BINARIES_DIR}/kube-apiserver" || sudo chmod 755 "${BINARIES_DIR}/kube-apiserver"
    
    echo "✓ kube-apiserver installed successfully"
    return 0
}

# Function to download kubectl
download_kubectl() {
    echo "==> Downloading kubectl version ${K8S_VERSION}"
    
    local KUBECTL_URL="https://dl.k8s.io/v${K8S_VERSION}/bin/linux/amd64/kubectl"
    if ! download_with_progress "${KUBECTL_URL}" "${BINARIES_DIR}/kubectl"; then
        echo "Trying with sudo..."
        if ! sudo curl -L --retry 5 --retry-delay 3 "${KUBECTL_URL}" -o "${BINARIES_DIR}/kubectl"; then
            echo "ERROR: Failed to download kubectl"
            return 1
        fi
    fi
    
    # Make binary executable with proper permissions (755)
    chmod 755 "${BINARIES_DIR}/kubectl" || sudo chmod 755 "${BINARIES_DIR}/kubectl"
    
    echo "✓ kubectl installed successfully"
    return 0
}

# Check for kubebuilder binary
if [ ! -f "${BINARIES_DIR}/kubebuilder" ] || [ ! -x "${BINARIES_DIR}/kubebuilder" ]; then
    echo "kubebuilder not found or not executable, downloading..."
    download_kubebuilder || {
        echo "ERROR: Failed to install kubebuilder"
        exit 1
    }
else
    echo "✓ kubebuilder already installed at ${BINARIES_DIR}/kubebuilder"
fi

# Check for etcd
if [ ! -f "${BINARIES_DIR}/etcd" ] || [ ! -x "${BINARIES_DIR}/etcd" ]; then
    echo "etcd not found or not executable, downloading..."
    download_etcd || {
        echo "ERROR: Failed to install etcd"
        exit 1
    }
else
    echo "✓ etcd already installed at ${BINARIES_DIR}/etcd"
fi

# Check for kube-apiserver
if [ ! -f "${BINARIES_DIR}/kube-apiserver" ] || [ ! -x "${BINARIES_DIR}/kube-apiserver" ]; then
    echo "kube-apiserver not found or not executable, downloading..."
    download_kube_apiserver || {
        echo "ERROR: Failed to install kube-apiserver"
        exit 1
    }
else
    echo "✓ kube-apiserver already installed at ${BINARIES_DIR}/kube-apiserver"
fi

# Check for kubectl
if [ ! -f "${BINARIES_DIR}/kubectl" ] || [ ! -x "${BINARIES_DIR}/kubectl" ]; then
    echo "kubectl not found or not executable, downloading..."
    download_kubectl || {
        echo "ERROR: Failed to install kubectl"
        exit 1
    }
else
    echo "✓ kubectl already installed at ${BINARIES_DIR}/kubectl"
fi

# Set environment variables
echo "==> Setting environment variables"
export PATH="${BINARIES_DIR}:${PATH}"
export KUBEBUILDER_ASSETS="${BINARIES_DIR}"

echo "==> Test binaries installation complete"
echo "KUBEBUILDER_ASSETS=${KUBEBUILDER_ASSETS}"
echo "PATH=${PATH}"

# Verify binaries are accessible
echo "==> Verifying binaries are accessible"
ls -la "${BINARIES_DIR}"

# Check kubebuilder
if [ -x "${BINARIES_DIR}/kubebuilder" ]; then
    echo "✓ kubebuilder found at ${BINARIES_DIR}/kubebuilder"
else
    echo "WARNING: kubebuilder not found or not executable"
fi

# Check etcd
if [ -x "${BINARIES_DIR}/etcd" ]; then
    "${BINARIES_DIR}/etcd" --version || echo "WARNING: etcd not working properly"
else
    echo "WARNING: etcd not found or not executable"
fi

# Check kube-apiserver
if [ -x "${BINARIES_DIR}/kube-apiserver" ]; then
    "${BINARIES_DIR}/kube-apiserver" --version || echo "WARNING: kube-apiserver not working properly"
else
    echo "WARNING: kube-apiserver not found or not executable"
fi

# Check kubectl
if [ -x "${BINARIES_DIR}/kubectl" ]; then
    "${BINARIES_DIR}/kubectl" version --client || echo "WARNING: kubectl not working properly"
else
    echo "WARNING: kubectl not found or not executable"
fi

echo "==> All necessary test binaries have been installed or verified"
