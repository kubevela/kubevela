#!/usr/bin/env bash

# This script downloads and installs the binaries required for running tests
set -e

# Set the binaries directory
BINARIES_DIR=${KUBEBUILDER_ASSETS:-/usr/local/kubebuilder/bin}
ETCD_VERSION=${ETCD_VERSION:-3.5.7}
K8S_VERSION=${K8S_VERSION:-1.26.1}

# Function to download a file with progress
download_with_progress() {
    local url=$1
    local output=$2
    echo "Downloading $url to $output"
    curl -L --progress-bar --retry 5 --retry-delay 3 "$url" -o "$output"
}

# Create the binaries directory if it doesn't exist
mkdir -p "${BINARIES_DIR}" || sudo mkdir -p "${BINARIES_DIR}"

# Ensure binaries directory is writable
if [ ! -w "${BINARIES_DIR}" ]; then
    echo "Making ${BINARIES_DIR} writable with sudo"
    sudo chmod -R 777 "${BINARIES_DIR}"
fi

# Function to download etcd
download_etcd() {
    echo "Downloading etcd version ${ETCD_VERSION}"
    
    local TMP_DIR=$(mktemp -d)
    pushd "${TMP_DIR}" > /dev/null
    
    local ETCD_URL="https://github.com/etcd-io/etcd/releases/download/v${ETCD_VERSION}/etcd-v${ETCD_VERSION}-linux-amd64.tar.gz"
    download_with_progress "${ETCD_URL}" "etcd.tar.gz"
    
    echo "Extracting etcd binaries"
    tar -xzf etcd.tar.gz --strip-components=1
    
    # Copy binaries to the target directory
    echo "Installing etcd binaries to ${BINARIES_DIR}"
    cp etcd "${BINARIES_DIR}/etcd" || sudo cp etcd "${BINARIES_DIR}/etcd"
    cp etcdctl "${BINARIES_DIR}/etcdctl" || sudo cp etcdctl "${BINARIES_DIR}/etcdctl"
    
    # Make binaries executable
    chmod +x "${BINARIES_DIR}/etcd" || sudo chmod +x "${BINARIES_DIR}/etcd"
    chmod +x "${BINARIES_DIR}/etcdctl" || sudo chmod +x "${BINARIES_DIR}/etcdctl"
    
    popd > /dev/null
    rm -rf "${TMP_DIR}"
    
    echo "✓ etcd binaries installed successfully"
}

# Function to download kube-apiserver
download_kube_apiserver() {
    echo "Downloading kube-apiserver version ${K8S_VERSION}"
    
    local API_SERVER_URL="https://dl.k8s.io/v${K8S_VERSION}/bin/linux/amd64/kube-apiserver"
    download_with_progress "${API_SERVER_URL}" "${BINARIES_DIR}/kube-apiserver" || \
        sudo curl -L --retry 5 --retry-delay 3 "${API_SERVER_URL}" -o "${BINARIES_DIR}/kube-apiserver"
    
    # Make binary executable
    chmod +x "${BINARIES_DIR}/kube-apiserver" || sudo chmod +x "${BINARIES_DIR}/kube-apiserver"
    
    echo "✓ kube-apiserver installed successfully"
}

# Function to download kubectl
download_kubectl() {
    echo "Downloading kubectl version ${K8S_VERSION}"
    
    local KUBECTL_URL="https://dl.k8s.io/v${K8S_VERSION}/bin/linux/amd64/kubectl"
    download_with_progress "${KUBECTL_URL}" "${BINARIES_DIR}/kubectl" || \
        sudo curl -L --retry 5 --retry-delay 3 "${KUBECTL_URL}" -o "${BINARIES_DIR}/kubectl"
    
    # Make binary executable
    chmod +x "${BINARIES_DIR}/kubectl" || sudo chmod +x "${BINARIES_DIR}/kubectl"
    
    echo "✓ kubectl installed successfully"
}

# Check for etcd
if [ ! -f "${BINARIES_DIR}/etcd" ] || [ ! -x "${BINARIES_DIR}/etcd" ]; then
    echo "etcd not found or not executable, downloading..."
    download_etcd
else
    echo "✓ etcd already installed at ${BINARIES_DIR}/etcd"
fi

# Check for kube-apiserver
if [ ! -f "${BINARIES_DIR}/kube-apiserver" ] || [ ! -x "${BINARIES_DIR}/kube-apiserver" ]; then
    echo "kube-apiserver not found or not executable, downloading..."
    download_kube_apiserver
else
    echo "✓ kube-apiserver already installed at ${BINARIES_DIR}/kube-apiserver"
fi

# Check for kubectl
if [ ! -f "${BINARIES_DIR}/kubectl" ] || [ ! -x "${BINARIES_DIR}/kubectl" ]; then
    echo "kubectl not found or not executable, downloading..."
    download_kubectl
else
    echo "✓ kubectl already installed at ${BINARIES_DIR}/kubectl"
fi

# Set environment variables
echo "Setting environment variables"
export PATH="${BINARIES_DIR}:${PATH}"
export KUBEBUILDER_ASSETS="${BINARIES_DIR}"

echo "Test binaries installation complete"
echo "KUBEBUILDER_ASSETS=${KUBEBUILDER_ASSETS}"
echo "PATH=${PATH}"

# Verify binaries are accessible
echo "Verifying binaries are accessible"
ls -la "${BINARIES_DIR}"
${BINARIES_DIR}/etcd --version || echo "etcd not working properly"
${BINARIES_DIR}/kube-apiserver --version || echo "kube-apiserver not working properly"
${BINARIES_DIR}/kubectl version --client || echo "kubectl not working properly"
