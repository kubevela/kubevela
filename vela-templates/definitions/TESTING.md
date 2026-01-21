# KubeVela Definition Testing Guide

This document describes the testing approaches for KubeVela component definitions and workflow steps.

## Quick Start

```bash
# From the definitions directory:

# 1. Run unit tests (fastest, no dependencies)
./test-apps/run-unit-tests.sh

# 2. Run integration tests (requires vela CLI)
./test-apps/run-integration-tests.sh

# 3. Run E2E tests (requires Kubernetes cluster with KubeVela)
./test-apps/e2e-test.sh
```

## Test Pyramid Overview

```
        /\
       /  \      E2E Tests (e2e-test.sh)
      /    \     - Real cluster deployment
     /------\    - Full system verification
    /        \
   / Integr-  \  Integration Tests (run-integration-tests.sh)
  / ation Tests\ - vela CLI validation
 /--------------\- Dry-run verification
/                \
/   Unit Tests    \ Unit Tests (Go + txtar)
/  (Go harness)   / - Fast feedback
-------------------  - Isolated testing
```

## 1. Unit Tests (Fastest)

### Go Tests with Txtar

**Location:** `internal/component/*_test.go` and `internal/workflowstep/*_test.go`

**Purpose:** Test CUE template rendering logic in isolation using the txtar format

**Run:**
```bash
# From definitions directory
./test-apps/run-unit-tests.sh

# Or run individual tests
cd internal/component && go test -v
cd internal/workflowstep && go test -v
```

**Files:**
- `statefulset_test.go` - Go test harness for statefulset component
- `testdata/statefulset.txtar` - Test data with input (in.cue) and expected output (out.cue)
- `build-push-image_test.go` - Go test harness for build-push-image workflow step
- `testdata/build-push-image.txtar` - Test data for workflow step

**Pros:**
- ✓ Very fast (<1s per test)
- ✓ No external dependencies (just Go and CUE)
- ✓ Easy to debug with clear diffs
- ✓ Great for TDD
- ✓ Tests actual CUE template rendering

**Cons:**
- ✗ Doesn't test actual Kubernetes behavior
- ✗ Mock data may differ from reality

## 2. Integration Tests (Medium Speed)

**Location:** `test-apps/run-integration-tests.sh`

**Purpose:** Validate definitions work with vela CLI and test complete integration flow

**Run:**
```bash
# From definitions directory
./test-apps/run-integration-tests.sh
```

**What it tests:**
- ✓ Go unit tests for component and workflow step definitions
- ✓ Definition exists and can be loaded in cluster (if KubeVela available)
- ✓ `vela dry-run` succeeds for test applications
- ✓ Definition metadata and schemas are correct

**Test Applications:**
- `test-apps/statefulset-integration-test.yaml` - StatefulSet component test
- `test-apps/build-push-integration-test.yaml` - Build-push-image workflow test

**Pros:**
- ✓ Tests vela CLI integration
- ✓ Catches definition loading errors
- ✓ No cluster required for dry-run mode
- ✓ Fast enough for CI
- ✓ Runs unit tests as part of integration flow

**Cons:**
- ✗ Doesn't test actual deployment
- ✗ Requires vela CLI for full validation (gracefully skips if unavailable)

## 3. E2E Tests (Slowest, Most Realistic)

**Location:** `test-apps/e2e-test.sh`

**Purpose:** Deploy actual applications to a Kubernetes cluster and verify end-to-end behavior

**Prerequisites:**
```bash
# Install KubeVela on your cluster
vela install

# Verify installation
kubectl get crd applications.core.oam.dev
```

**Run:**
```bash
# From definitions directory
./test-apps/e2e-test.sh

# Or with custom settings
TEST_NAMESPACE=my-test TIMEOUT=600 ./test-apps/e2e-test.sh
```

**Test Scenarios:**
1. **StatefulSet Component Deployment**
   - Deploys PostgreSQL database using statefulset component
   - Verifies StatefulSet, Service, and Pod creation
   - Tests database connectivity with retry logic (handles async initialization)
   - Validates resource limits are applied correctly

2. **Build-Push-Image Workflow (Dry-Run)**
   - Validates workflow step definition
   - Tests vela dry-run with workflow steps
   - Gracefully skips if webhook unavailable

**Pros:**
- ✓ Tests real behavior in actual cluster
- ✓ Catches integration and deployment issues
- ✓ Validates full application lifecycle
- ✓ Tests actual Kubernetes resource creation
- ✓ Resilient to async operations (retry logic)
- ✓ Graceful degradation for missing dependencies

**Cons:**
- ✗ Slower (1-3 minutes per test)
- ✗ Requires running Kubernetes cluster with KubeVela
- ✗ Resource intensive

## Running Tests

All commands assume you're in the `definitions` directory.

### Quick Test Commands

```bash
# Run unit tests only (fastest, no dependencies)
./test-apps/run-unit-tests.sh

# Run integration tests (requires vela CLI, optional)
./test-apps/run-integration-tests.sh

# Run E2E tests (requires Kubernetes cluster with KubeVela)
./test-apps/e2e-test.sh

# Run specific component test
cd internal/component && go test -v -run TestStatefulSet

# Run specific workflow step test
cd internal/workflowstep && go test -v -run TestBuildPushImage
```

## CI/CD Pipeline Recommendation

Example GitHub Actions workflow for comprehensive testing:

```yaml
# .github/workflows/test.yml
name: Test KubeVela Definitions

on: [push, pull_request]

jobs:
  unit-tests:
    name: Unit Tests
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'

      - uses: cue-lang/setup-cue@v1.0.1

      - name: Run unit tests
        run: |
          cd definitions
          ./test-apps/run-unit-tests.sh

  integration-tests:
    name: Integration Tests
    runs-on: ubuntu-latest
    needs: unit-tests
    steps:
      - uses: actions/checkout@v4

      - name: Install vela CLI
        run: |
          curl -fsSl https://kubevela.io/script/install.sh | bash

      - name: Run integration tests
        run: |
          cd definitions
          ./test-apps/run-integration-tests.sh

  e2e-tests:
    name: E2E Tests
    runs-on: ubuntu-latest
    needs: integration-tests
    steps:
      - uses: actions/checkout@v4
      - uses: helm/kind-action@v1.8.0
        with:
          cluster_name: vela-test

      - name: Install KubeVela
        run: |
          curl -fsSl https://kubevela.io/script/install.sh | bash
          vela install --wait

      - name: Run E2E tests
        run: |
          cd definitions
          ./test-apps/e2e-test.sh
```

**Pipeline Notes:**
- Unit tests run first (fast feedback)
- Integration tests require vela CLI but no cluster
- E2E tests require full Kubernetes cluster with KubeVela
- Each stage depends on previous stage passing

## Test Coverage Matrix

| Test Type    | Speed   | Isolation | Reality | CI Friendly | Dependencies | Cost |
|--------------|---------|-----------|---------|-------------|--------------|------|
| Unit (Go)    | ⚡⚡⚡ | ✓✓✓       | ✗       | ✓✓✓         | Go + CUE     | $    |
| Integration  | ⚡⚡    | ✓✓        | ✓       | ✓✓          | vela CLI     | $$   |
| E2E          | ⚡      | ✗         | ✓✓✓     | ✓           | K8s + vela   | $$$  |

## Best Practices

1. **Write unit tests first** - Fast feedback during development with txtar format
2. **Test template rendering logic** - Use Go tests with CUE unification to validate output
3. **Run tests locally before push** - Unit tests are fast and catch most issues
4. **Integration tests validate CLI integration** - Ensure definitions work with vela CLI
5. **E2E tests for critical paths** - Verify real behavior in actual cluster
6. **Keep E2E tests focused** - Test one component or workflow per test
7. **Use txtar for test data** - Standard CUE testing convention
8. **Handle async operations** - Add retry logic for services with initialization time
9. **Graceful degradation** - Skip tests when optional dependencies unavailable

## Adding New Tests

### For a New Component Definition:

1. **Create unit test** - `internal/component/mycomponent_test.go`
   ```go
   func TestMyComponent(t *testing.T) {
       RunTest(t, "testdata/mycomponent.txtar")
   }
   ```

2. **Create test data** - `internal/component/testdata/mycomponent.txtar`
   ```
   -- in.cue --
   context: {...}
   parameter: {...}
   -- out.cue --
   output: {...}
   outputs: {...}
   ```

3. **Create integration test app** - `test-apps/mycomponent-integration-test.yaml`
   - Sample Application using your component

4. **Add E2E test scenario** (if critical)
   - Add test function to `test-apps/e2e-test.sh`
   - Deploy real application and verify behavior

### For a New Workflow Step:

1. **Create unit test** - `internal/workflowstep/mystep_test.go`
   ```go
   func TestMyStep(t *testing.T) {
       RunTest(t, "testdata/mystep.txtar")
   }
   ```

2. **Create test data** - `internal/workflowstep/testdata/mystep.txtar`
   - Include input parameters and expected output

3. **Create integration test app** - `test-apps/mystep-integration-test.yaml`
   - Application with workflow using your step

4. **Consider E2E test** if the step:
   - Modifies cluster state
   - Interacts with external systems
   - Has complex runtime behavior

## Troubleshooting

### Unit Tests Failing

**CUE rendering errors:**
```bash
# Test CUE rendering manually
cd internal/component
cue eval -c statefulset.cue testdata/statefulset.txtar

# Check for syntax errors
cue fmt statefulset.cue
```

**Unification failures:**
```bash
# Run test with verbose output
go test -v -run TestStatefulSet

# Check txtar file structure
cat testdata/statefulset.txtar
```

### Integration Tests Failing

**Definition not loading:**
```bash
# Check if KubeVela is installed
kubectl get crd applications.core.oam.dev

# Verify definition can be loaded
vela def get statefulset

# Check for CUE syntax errors in definition
cue vet internal/component/statefulset.cue
```

**Dry-run failing:**
```bash
# Run vela dry-run manually
vela dry-run -f test-apps/statefulset-integration-test.yaml

# Check application YAML syntax
kubectl apply --dry-run=client -f test-apps/statefulset-integration-test.yaml
```

### E2E Tests Failing

**Application not deploying:**
```bash
# Check application status and conditions
kubectl get app -n vela-test
kubectl describe app <app-name> -n vela-test

# Check application revision
kubectl get apprev -n vela-test
```

**Pods not starting:**
```bash
# Check pod status
kubectl get pods -n vela-test

# View pod events
kubectl describe pod <pod-name> -n vela-test

# Check pod logs
kubectl logs -n vela-test <pod-name>
```

**Resource creation failing:**
```bash
# List all resources created by application
kubectl get all -n vela-test -l app.oam.dev/name=<app-name>

# Check KubeVela controller logs
kubectl logs -n vela-system -l app.kubernetes.io/name=vela-core
```

**Test timing out:**
```bash
# Increase timeout
TIMEOUT=600 ./test-apps/e2e-test.sh

# Check if services are slow to initialize
kubectl logs -n vela-test <pod-name> --tail=50
```

## Test Files Overview

```
definitions/
├── internal/
│   ├── component/
│   │   ├── statefulset.cue              # Component definition
│   │   ├── statefulset_test.go          # Go unit test
│   │   └── testdata/
│   │       └── statefulset.txtar        # Test data (in.cue + out.cue)
│   └── workflowstep/
│       ├── build-push-image.cue         # Workflow step definition
│       ├── build-push-image_test.go     # Go unit test
│       └── testdata/
│           └── build-push-image.txtar   # Test data
└── test-apps/
    ├── run-unit-tests.sh                # Run all Go unit tests
    ├── run-integration-tests.sh         # Run integration tests with vela CLI
    ├── e2e-test.sh                      # Run E2E tests on cluster
    ├── statefulset-integration-test.yaml
    └── build-push-integration-test.yaml
```

## Resources

- [CUE Testing Guide](https://cuelang.org/docs/integrations/go/)
- [CUE Txtar Format](https://pkg.go.dev/golang.org/x/tools/txtar)
- [KubeVela Testing](https://kubevela.io/docs/contributor/testing)
- [KubeVela Definition CRD](https://kubevela.io/docs/platform-engineers/cue/definition-edit)

## Summary

This testing infrastructure provides three levels of testing:

1. **Unit Tests** - Fast, isolated tests using Go + txtar format (seconds)
2. **Integration Tests** - Validation with vela CLI (minutes)
3. **E2E Tests** - Full cluster deployment tests (minutes)

All tests are designed with:
- ✅ Clear pass/fail criteria
- ✅ Helpful error messages
- ✅ Retry logic for async operations
- ✅ Graceful degradation when dependencies unavailable
- ✅ CI/CD friendly design

Start with unit tests for rapid development, use integration tests to validate CLI behavior, and run E2E tests before releases to ensure production readiness.
