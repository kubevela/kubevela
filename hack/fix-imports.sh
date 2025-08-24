#!/usr/bin/env bash

# This script fixes import-related build errors in the codebase

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

echo "Fixing import-related build errors"

# Fix unused imports using goimports
if command -v goimports &> /dev/null; then
    echo "==> Using goimports to fix imports"
    find "${ROOT_DIR}" -name "*.go" -not -path "*/vendor/*" -not -path "*/bin/*" -exec goimports -w {} \;
else
    echo "==> goimports not found, installing..."
    go install golang.org/x/tools/cmd/goimports@latest
    find "${ROOT_DIR}" -name "*.go" -not -path "*/vendor/*" -not -path "*/bin/*" -exec goimports -w {} \;
fi

# Fix specific issues in pkg/rollout/suit_test.go
echo "==> Fixing specific issues in pkg/rollout/suit_test.go"
if [ -f "${ROOT_DIR}/pkg/rollout/suit_test.go" ]; then
    # Remove the unused os import
    sed -i '/^import (/,/)/ { /^\s*"os"$/d }' "${ROOT_DIR}/pkg/rollout/suit_test.go"
    echo "✓ Fixed unused import in pkg/rollout/suit_test.go"
else
    echo "⚠️ File not found: pkg/rollout/suit_test.go"
fi

echo "Import fixes complete"
