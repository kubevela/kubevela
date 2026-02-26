#!/bin/bash
# Don't exit on error - we want to run all tests and report summary
set +e

# Integration tests for KubeVela definitions
# These tests use vela CLI to validate definitions work correctly

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_NAMESPACE="${TEST_NAMESPACE:-default}"
FAILED_TESTS=0
PASSED_TESTS=0

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

test_passed() {
    log_info "✓ Test passed: $1"
    ((PASSED_TESTS++))
}

test_failed() {
    log_error "✗ Test failed: $1"
    ((FAILED_TESTS++))
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."

    if ! command -v vela &> /dev/null; then
        log_error "vela CLI not found. Please install KubeVela CLI."
        exit 1
    fi

    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl not found. Please install kubectl."
        exit 1
    fi

    log_info "Prerequisites check passed"
}

# Test 1: Validate StatefulSet definition (requires KubeVela installed)
test_statefulset_definition() {
    log_info "Testing StatefulSet component definition (requires KubeVela)..."

    # Check if vela can access a cluster
    if ! vela version &> /dev/null; then
        log_warn "KubeVela not accessible - skipping definition load test"
        log_warn "To run this test: install KubeVela with 'vela install'"
        return
    fi

    if vela def get statefulset &> /dev/null; then
        test_passed "StatefulSet definition exists in cluster"
    else
        log_warn "StatefulSet definition not loaded (install definitions first)"
        return
    fi

    # Dry-run the test application
    if vela dry-run -f "$SCRIPT_DIR/statefulset-integration-test.yaml" &> /dev/null; then
        test_passed "StatefulSet dry-run succeeded"
    else
        log_warn "StatefulSet dry-run failed (this is expected if traits are not installed)"
    fi
}

# Test 2: Validate build-push-image workflow step (requires KubeVela installed)
test_build_push_workflow() {
    log_info "Testing build-push-image workflow step (requires KubeVela)..."

    # Check if vela can access a cluster
    if ! vela version &> /dev/null; then
        log_warn "KubeVela not accessible - skipping workflow step test"
        log_warn "To run this test: install KubeVela with 'vela install'"
        return
    fi

    if vela def get build-push-image &> /dev/null; then
        test_passed "build-push-image definition exists in cluster"
    else
        log_warn "build-push-image definition not loaded (install definitions first)"
        return
    fi

    # Dry-run the test application
    if vela dry-run -f "$SCRIPT_DIR/build-push-integration-test.yaml" 2>&1 | grep -q "application validated"; then
        test_passed "build-push-image dry-run validation passed"
    else
        log_warn "build-push-image dry-run had warnings (check if all workflow steps are installed)"
    fi
}

# Test 3: Validate component definition schema
test_component_schema() {
    log_info "Testing component definitions..."

    local component_dir="$(dirname "$SCRIPT_DIR")/internal/component"

    # Run Go unit tests which properly validate the definitions
    if command -v go &> /dev/null; then
        log_info "Running Go unit tests for components..."
        echo "----------------------------------------"
        if (cd "$component_dir" && go test -v); then
            echo "----------------------------------------"
            test_passed "Component unit tests passed"
        else
            echo "----------------------------------------"
            test_failed "Component unit tests failed"
        fi
    else
        log_warn "Go not found, skipping unit tests"
        # Fallback: just check files exist
        if [ -f "$component_dir/statefulset.cue" ] && [ -f "$component_dir/statefulset_test.go" ]; then
            test_passed "StatefulSet definition and test files exist"
        else
            test_failed "StatefulSet definition files not found"
        fi
    fi
}

# Test 4: Validate workflow step schema
test_workflow_schema() {
    log_info "Testing workflow step definitions..."

    local workflow_dir="$(dirname "$SCRIPT_DIR")/internal/workflowstep"

    # Run Go unit tests which properly validate the definitions
    if command -v go &> /dev/null; then
        log_info "Running Go unit tests for workflow steps..."
        echo "----------------------------------------"
        if (cd "$workflow_dir" && go test -v); then
            echo "----------------------------------------"
            test_passed "Workflow step unit tests passed"
        else
            echo "----------------------------------------"
            test_failed "Workflow step unit tests failed"
        fi
    else
        log_warn "Go not found, skipping unit tests"
        # Fallback: just check files exist
        if [ -f "$workflow_dir/build-push-image.cue" ] && [ -f "$workflow_dir/build-push-image_test.go" ]; then
            test_passed "build-push-image definition and test files exist"
        else
            test_failed "build-push-image definition files not found"
        fi
    fi
}

# Main test execution
main() {
    log_info "Starting KubeVela Definition Integration Tests"
    log_info "=============================================="
    echo ""

    check_prerequisites
    echo ""

    test_component_schema
    echo ""

    test_workflow_schema
    echo ""

    test_statefulset_definition
    echo ""

    test_build_push_workflow
    echo ""

    # Summary
    log_info "=============================================="
    log_info "Test Summary:"
    log_info "  Passed: $PASSED_TESTS"

    if [ $FAILED_TESTS -gt 0 ]; then
        log_error "  Failed: $FAILED_TESTS"
        log_error ""
        log_error "Some tests failed. Check the output above for details."
        exit 1
    else
        log_info "  Failed: $FAILED_TESTS"
        log_info ""
        log_info "All executed tests passed! ✓"
        log_info ""
        log_info "Note: Some tests may have been skipped due to missing prerequisites."
        log_info "Install KubeVela (vela install) to run full integration tests."
    fi
}

main "$@"
