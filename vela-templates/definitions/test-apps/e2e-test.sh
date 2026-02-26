#!/bin/bash
set -e

# End-to-End tests for KubeVela definitions
# These tests deploy actual applications to a cluster and verify behavior

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_NAMESPACE="${TEST_NAMESPACE:-vela-test}"
TIMEOUT="${TIMEOUT:-300}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_step() {
    echo -e "${BLUE}[STEP]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Cleanup function
cleanup() {
    log_info "Cleaning up test resources..."
    kubectl delete namespace "$TEST_NAMESPACE" --ignore-not-found=true --wait=false 2>/dev/null || true
}

# Register cleanup on exit
trap cleanup EXIT

# Setup test environment
setup_test_env() {
    log_step "Setting up test environment..."

    # Create test namespace
    kubectl create namespace "$TEST_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

    # Wait for namespace to be ready
    kubectl wait --for=jsonpath='{.status.phase}'=Active namespace/"$TEST_NAMESPACE" --timeout=30s

    log_info "Test namespace '$TEST_NAMESPACE' ready"
}

# E2E Test 1: Deploy StatefulSet component
test_e2e_statefulset() {
    log_step "E2E Test: StatefulSet Component Deployment"

    local app_name="e2e-statefulset-test"
    local app_file="$SCRIPT_DIR/e2e-statefulset.yaml"

    # Create test application manifest
    cat > "$app_file" <<EOF
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: $app_name
  namespace: $TEST_NAMESPACE
spec:
  components:
    - name: test-db
      type: statefulset
      properties:
        image: postgres:16.4
        cpu: "0.5"
        memory: 512Mi
        exposeType: ClusterIP
        ports:
          - port: 5432
            protocol: TCP
            expose: true
        env:
          - name: POSTGRES_DB
            value: e2etest
          - name: POSTGRES_USER
            value: testuser
          - name: POSTGRES_PASSWORD
            value: testpass123
EOF

    # Deploy application
    log_info "Deploying application..."
    kubectl apply -f "$app_file"

    # Wait for application to be ready
    log_info "Waiting for application to be running (timeout: ${TIMEOUT}s)..."
    if kubectl wait --for=condition=Ready application/"$app_name" \
        -n "$TEST_NAMESPACE" --timeout="${TIMEOUT}s" 2>/dev/null; then
        log_info "✓ Application deployed successfully"
    else
        log_error "✗ Application failed to deploy"
        kubectl get application "$app_name" -n "$TEST_NAMESPACE" -o yaml
        return 1
    fi

    # Verify StatefulSet was created
    log_info "Verifying StatefulSet creation..."
    if kubectl get statefulset -n "$TEST_NAMESPACE" -l "app.oam.dev/component=test-db" | grep -q "test-db"; then
        log_info "✓ StatefulSet created"
    else
        log_error "✗ StatefulSet not found"
        return 1
    fi

    # Verify Service was created (for exposed ports)
    log_info "Verifying Service creation..."
    if kubectl get service -n "$TEST_NAMESPACE" "test-db" 2>/dev/null; then
        log_info "✓ Service created"
    else
        log_error "✗ Service not found"
        return 1
    fi

    # Verify Pod is running
    log_info "Verifying Pod status..."
    if kubectl wait --for=condition=Ready pod \
        -l "app.oam.dev/component=test-db" \
        -n "$TEST_NAMESPACE" --timeout=120s 2>/dev/null; then
        log_info "✓ Pod is running and ready"
    else
        log_error "✗ Pod failed to become ready"
        kubectl get pods -n "$TEST_NAMESPACE" -l "app.oam.dev/component=test-db"
        return 1
    fi

    # Test database connectivity with retry (PostgreSQL needs time to initialize)
    log_info "Testing database connectivity..."
    local pod_name=$(kubectl get pod -n "$TEST_NAMESPACE" -l "app.oam.dev/component=test-db" -o jsonpath='{.items[0].metadata.name}')

    # Wait for PostgreSQL to be ready (it takes a few seconds after pod is ready)
    log_info "Waiting for PostgreSQL to initialize..."
    sleep 10

    # Retry logic for database connectivity
    local max_retries=5
    local retry=0
    local connected=false

    while [ $retry -lt $max_retries ]; do
        if kubectl exec -n "$TEST_NAMESPACE" "$pod_name" -- psql -U testuser -d e2etest -c "SELECT 1" &>/dev/null; then
            log_info "✓ Database is accessible and responding"
            connected=true
            break
        else
            log_info "Database not ready yet, retrying... ($((retry+1))/$max_retries)"
            sleep 5
            ((retry++))
        fi
    done

    if [ "$connected" = false ]; then
        log_error "✗ Database connectivity test failed after $max_retries attempts"
        log_info "Checking database logs..."
        kubectl logs -n "$TEST_NAMESPACE" "$pod_name" --tail=20
        return 1
    fi

    # Verify resource limits were applied
    log_info "Verifying resource limits..."
    local cpu_limit=$(kubectl get pod -n "$TEST_NAMESPACE" "$pod_name" -o jsonpath='{.spec.containers[0].resources.limits.cpu}')
    local memory_limit=$(kubectl get pod -n "$TEST_NAMESPACE" "$pod_name" -o jsonpath='{.spec.containers[0].resources.limits.memory}')

    log_info "  CPU limit: $cpu_limit"
    log_info "  Memory limit: $memory_limit"

    if [ "$cpu_limit" == "0.5" ] || [ "$cpu_limit" == "500m" ]; then
        log_info "✓ CPU limit correctly applied"
    else
        log_error "✗ CPU limit incorrect. Expected: 0.5, Got: $cpu_limit"
        return 1
    fi

    # Cleanup
    log_info "Cleaning up test application..."
    kubectl delete application "$app_name" -n "$TEST_NAMESPACE" --wait=true

    log_info "✓ E2E StatefulSet test PASSED"
    return 0
}

# E2E Test 2: Verify build-push-image workflow (dry-run only)
test_e2e_build_push_workflow() {
    log_step "E2E Test: Build-Push-Image Workflow (Dry-Run)"

    local app_name="e2e-build-push-test"

    # Note: This is a dry-run test since we don't have actual git credentials
    log_info "Testing workflow step validation..."

    # Check if webhook is available
    if ! kubectl get endpoints -n vela-system vela-core-webhook &>/dev/null; then
        log_warn "KubeVela webhook not available - skipping workflow validation test"
        log_warn "This is normal if KubeVela webhook is not running"
        return 0
    fi

    local output
    output=$(vela dry-run -f "$SCRIPT_DIR/build-push-integration-test.yaml" 2>&1)
    local vela_exit=$?

    if echo "$output" | grep -q "no endpoints available"; then
        log_warn "✓ Workflow test skipped (KubeVela webhook service not available)"
        log_warn "To fix: ensure KubeVela webhook is running in vela-system namespace"
        return 0
    elif [ $vela_exit -eq 0 ]; then
        log_info "✓ Workflow validation passed"
        return 0
    else
        log_error "✗ Workflow validation failed"
        echo "$output"
        return 1
    fi
}

# Main execution
main() {
    log_info "=========================================="
    log_info "KubeVela E2E Tests"
    log_info "=========================================="
    echo ""

    # Check if running in a cluster with vela installed
    if ! kubectl get crd applications.core.oam.dev &>/dev/null; then
        log_error "KubeVela CRDs not found. Please install KubeVela first."
        log_error "Run: vela install"
        exit 1
    fi

    setup_test_env
    echo ""

    # Run E2E tests
    local failed=0

    if test_e2e_statefulset; then
        echo ""
    else
        ((failed++))
    fi

    if test_e2e_build_push_workflow; then
        echo ""
    else
        ((failed++))
    fi

    # Summary
    log_info "=========================================="
    if [ $failed -eq 0 ]; then
        log_info "All E2E tests PASSED ✓"
        exit 0
    else
        log_error "$failed E2E test(s) FAILED ✗"
        exit 1
    fi
}

main "$@"
