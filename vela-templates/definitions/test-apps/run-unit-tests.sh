#!/bin/bash
set -e

# Simple unit test runner for KubeVela definitions
# No external dependencies required (just Go and CUE)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FAILED_TESTS=0
PASSED_TESTS=0

GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

test_passed() {
    log_info "✓ $1"
    ((PASSED_TESTS++))
}

test_failed() {
    log_error "✗ $1"
    ((FAILED_TESTS++))
}

# Test components
test_components() {
    log_info "Testing component definitions..."
    local component_dir="$(dirname "$SCRIPT_DIR")/internal/component"

    if (cd "$component_dir" && go test -v); then
        test_passed "Component unit tests passed"
    else
        test_failed "Component unit tests failed"
    fi
}

# Test workflow steps
test_workflow_steps() {
    log_info "Testing workflow step definitions..."
    local workflow_dir="$(dirname "$SCRIPT_DIR")/internal/workflowstep"

    if (cd "$workflow_dir" && go test -v); then
        test_passed "Workflow step unit tests passed"
    else
        test_failed "Workflow step unit tests failed"
    fi
}

# Main
main() {
    log_info "=========================================="
    log_info "KubeVela Definition Unit Tests"
    log_info "=========================================="
    echo ""

    test_components
    echo ""

    test_workflow_steps
    echo ""

    # Summary
    log_info "=========================================="
    log_info "Test Summary:"
    log_info "  Passed: $PASSED_TESTS"

    if [ $FAILED_TESTS -gt 0 ]; then
        log_error "  Failed: $FAILED_TESTS"
        exit 1
    else
        log_info "  Failed: $FAILED_TESTS"
        log_info "All tests passed! ✓"
    fi
}

main "$@"
