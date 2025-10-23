#!/bin/bash

# Setup script for running integration tests
# This script installs and configures the required tools for running integration tests
# Usage: ./setup-integration-tests.sh [setup|cleanup]

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$( cd "$SCRIPT_DIR/.." && pwd )"
LOCALBIN="$PROJECT_ROOT/bin"
ENVTEST_K8S_VERSION="${ENVTEST_K8S_VERSION:-1.31.0}"

# Parse command
COMMAND="${1:-setup}"

# Function to cleanup envtest resources
cleanup_envtest() {
    echo "==> Cleaning up integration test environment..."

    # Remove setup-envtest binary
    if [ -f "$LOCALBIN/setup-envtest" ]; then
        echo "Removing setup-envtest binary..."
        rm -f "$LOCALBIN/setup-envtest"
    fi

    # Get the path where binaries are stored
    if [ -f "$LOCALBIN/setup-envtest" ]; then
        ASSETS_PATH=$("$LOCALBIN/setup-envtest" use "$ENVTEST_K8S_VERSION" -p path 2>/dev/null || echo "")
    else
        # Common paths where envtest stores binaries
        if [ "$(uname)" = "Darwin" ]; then
            ASSETS_PATH="$HOME/Library/Application Support/io.kubebuilder.envtest"
        else
            ASSETS_PATH="${XDG_DATA_HOME:-$HOME/.local/share}/kubebuilder-envtest"
        fi
    fi

    # Remove downloaded Kubernetes binaries
    if [ -n "$ASSETS_PATH" ] && [ -d "$ASSETS_PATH" ]; then
        echo "Removing Kubernetes test binaries from $ASSETS_PATH..."
        # Use sudo if needed for permission issues, but try without first
        if rm -rf "$ASSETS_PATH" 2>/dev/null; then
            echo "Removed test binaries successfully"
        else
            echo "Note: Some files may require elevated permissions to remove."
            echo "You may need to manually remove: $ASSETS_PATH"
            echo "Or run: sudo rm -rf \"$ASSETS_PATH\""
        fi
    fi

    echo "Cleanup complete!"
    exit 0
}

# Handle cleanup command
if [ "$COMMAND" = "cleanup" ] || [ "$COMMAND" = "clean" ]; then
    cleanup_envtest
fi

echo "==> Setting up integration test environment..."

# Create bin directory
mkdir -p "$LOCALBIN"

# Install setup-envtest if not present
if [ ! -f "$LOCALBIN/setup-envtest" ]; then
    echo "==> Installing setup-envtest..."
    GOBIN="$LOCALBIN" go install sigs.k8s.io/controller-runtime/tools/setup-envtest@v0.0.0-20240522175850-2e9781e9fc60
    echo "setup-envtest installed"
else
    echo "setup-envtest already installed"
fi

# Download Kubernetes binaries for envtest
echo "==> Downloading Kubernetes ${ENVTEST_K8S_VERSION} binaries..."
KUBEBUILDER_ASSETS=$("$LOCALBIN/setup-envtest" use "$ENVTEST_K8S_VERSION" -p path)
echo "Kubernetes binaries installed at: $KUBEBUILDER_ASSETS"

# Export for immediate use
export KUBEBUILDER_ASSETS

echo ""
echo "==> Setup complete! You can now run integration tests with:"
echo ""
echo "  # Run only integration tests for server"
echo "  make integration-test-core"
echo ""
echo "  # Run all server tests (unit + integration)"
echo "  make test-server-all"
echo ""
echo "  # Or run manually with:"
echo "  KUBEBUILDER_ASSETS=\"$KUBEBUILDER_ASSETS\" go test -tags integration -v ./cmd/core/app -run TestIntegration"
echo ""