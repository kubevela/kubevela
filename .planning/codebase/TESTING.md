# KubeVela Testing Patterns

## Framework Overview

The codebase uses **two testing styles** side by side:
1. **Ginkgo v2 + Gomega** — for integration tests (envtest) and E2E tests
2. **Standard `go test` + testify/require or testify/assert** — for pure unit tests

Both styles coexist in `pkg/`. The rule of thumb: if a test needs a k8s API server, use Ginkgo + envtest. If it only exercises in-memory logic, use `go test` with `require`.

## Running Tests

### Unit Tests (all pkg + cmd + apis)
```bash
KUBEBUILDER_ASSETS="$(envtest use <version> -p path)" \
  go test -coverprofile=coverage.txt $(go list ./pkg/... ./cmd/... ./apis/...)
```
Target: `make unit-test-core` (from `Makefile` line 24)

### E2E Tests (against live cluster)
```bash
# Full E2E suite
ginkgo -v ./test/e2e-test

# Local dev with k3d (builds+loads image, deploys via Helm, runs ginkgo)
make e2e-test-local

# Multicluster E2E
cd ./test/e2e-multicluster-test && go test -timeout=30m -v -ginkgo.v -ginkgo.trace

# Addon E2E
ginkgo -v ./test/e2e-addon-test
```
See `makefiles/e2e.mk` for all E2E targets and cluster setup scripts.

### Linting
```bash
make lint          # golangci-lint via hack/utils/golangci-lint-wrapper.sh
make vet           # go vet ./...
make staticcheck   # staticcheck
make reviewable    # fmt + vet + lint + staticcheck + helm-doc-gen
```

## Ginkgo Test Structure

### Suite Bootstrap (`*_suite_test.go` / `suit_test.go`)
Every Ginkgo package has a bootstrap file that:
1. Calls `RegisterFailHandler(Fail)` and `RunSpecs(t, "Suite Name")`
2. Sets up `envtest.Environment` or connects to a live cluster in `BeforeSuite`
3. Tears down in `AfterSuite`

Example — envtest setup (`pkg/resourcekeeper/suite_test.go`):
```go
func TestResourceKeeper(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "ResourceKeeper Suite")
}
var _ = BeforeSuite(func() {
    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{filepath.Join("../..", "charts/vela-core/crds")},
        UseExistingCluster: ptr.To(false),
    }
    cfg, err := testEnv.Start()
    ...
    testClient, err = client.New(cfg, client.Options{Scheme: common.Scheme})
})
var _ = AfterSuite(func() { Expect(testEnv.Stop()).Should(Succeed()) })
```

Example — live cluster connection (`test/e2e-test/suite_test.go`):
```go
var _ = BeforeSuite(func() {
    k8sClient, err = client.New(config.GetConfigOrDie(), client.Options{Scheme: scheme})
})
```

### Describe/Context/It Pattern
All E2E and integration test cases follow standard Ginkgo BDD hierarchy:
```go
var _ = Describe("PostDispatch Trait tests", func() {
    ctx := context.Background()
    var namespace string

    BeforeEach(func() {
        namespace = randomNamespaceName("postdispatch-test")
        Expect(k8sClient.Create(ctx, &corev1.Namespace{...})).Should(Succeed())
    })

    AfterEach(func() {
        Expect(k8sClient.Delete(ctx, ns)).Should(Succeed())
    })

    Context("Test PostDispatch status", func() {
        It("Should mark application healthy", func() {
            By("Creating PostDispatch deployment trait definition")
            ...
            Eventually(func() error { ... }, timeout, interval).Should(Succeed())
        })
    })
})
```
Source: `test/e2e-test/postdispatch_trait_test.go`, `test/e2e-test/trait_test.go`

### By() Usage
`By(...)` is used liberally as inline documentation to label test steps:
```go
By("Bootstrapping test environment")
By("Setting up kubernetes client")
By("Created deployments.apps")
By("Cleaning up test namespace")
```

### Eventually / Consistently
All async Kubernetes operations use `Eventually` with explicit timeout and poll interval:
```go
Eventually(
    func() error { return k8sClient.Get(ctx, key, obj) },
    time.Second*120, time.Millisecond*500,
).Should(&util.NotFoundMatcher{})
```
Common timeouts: 120s for namespace deletion, 600s for pod readiness in E2E cluster setup.

### Custom Matchers
Located in `pkg/oam/util/`:
- `util.AlreadyExistMatcher{}` — matches `IsAlreadyExists` API errors (used with `SatisfyAny(BeNil(), ...)`)
- `util.NotFoundMatcher{}` — matches `IsNotFound` API errors

Usage pattern:
```go
Expect(k8sClient.Create(ctx, obj)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
```
Source: `test/e2e-test/suite_test.go:105,122,142`

## Unit Test Style (go test + testify)

### Table-Driven Tests with Map Keys
KubeVela uses `map[string]struct{}` for table-driven tests (not slice), which gives random execution order — this is intentional for isolation:
```go
func TestParseOverridePolicyRelatedDefinitions(t *testing.T) {
    testCases := map[string]struct {
        Policy        v1beta1.AppPolicy
        ComponentDefs []*v1beta1.ComponentDefinition
        Error         string
    }{
        "normal":                 { ... },
        "invalid-override-policy": { ... },
        "comp-def-not-found":     { ... },
    }
    for name, tc := range testCases {
        t.Run(name, func(t *testing.T) { ... })
    }
}
```
Source: `pkg/policy/override_test.go`

### testify/require Pattern
```go
func TestResourceKeeperDispatchAndDelete(t *testing.T) {
    r := require.New(t)
    cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
    ...
    r.NoError(err)
    r.Equal(2, len(rk._rootRT.Spec.ManagedResources))
}
```
Source: `pkg/resourcekeeper/dispatch_and_delete_test.go`

### testify/assert Pattern (less common)
Used in `pkg/oam/util/helper_test.go` alongside `apierrors` for error type checking.

## Fake Clients

### controller-runtime fake client
```go
cli := fake.NewClientBuilder().
    WithScheme(common.Scheme).
    WithObjects(obj1, obj2).
    Build()
```
Used in: `pkg/policy/override_test.go`, `pkg/resourcekeeper/dispatch_and_delete_test.go`

### Custom mock client
`pkg/oam/mock/client.go` wraps the fake client to override `RESTMapper` with custom GVK→GVR mappings. Used when tests need specific REST mapping behavior.

## Integration Tests with envtest

Packages using `envtest.Environment` (not live cluster):
- `pkg/resourcekeeper/` — `suite_test.go` starts two envtest environments (hub + worker)
- `pkg/controller/core.oam.dev/v1beta1/application/` — `suite_test.go` starts envtest + spins up controller manager with `ctrl.NewManager`
- `pkg/addon/`, `pkg/config/`, `pkg/velaql/`, `pkg/workflow/` — each has a `*_suite_test.go`

CRD path for all envtest suites: `filepath.Join("../..", "charts/vela-core/crds")`

## E2E Test Infrastructure

### Cluster Setup
E2E tests (`test/e2e-test/`) connect to an **existing cluster** (not envtest). The `BeforeSuite` calls `config.GetConfigOrDie()`.

Cluster is provisioned by:
```bash
make e2e-setup-core       # helm install kubevela + addon setup
make e2e-setup-core-auth  # same but with authentication + sharding enabled
```

### Namespace Isolation
Each `It` block runs in an isolated random namespace created in `BeforeEach` and deleted in `AfterEach`:
```go
namespace = randomNamespaceName("trait-test")
// randomNamespaceName generates: "trait-test-<random-hex>"
```
Source: `test/e2e-test/suite_test.go:177`

### Requesting Immediate Reconcile
```go
func RequestReconcileNow(ctx context.Context, o client.Object) {
    // Patches a timestamp annotation to force immediate reconcile
    oMeta.SetAnnotations(map[string]string{
        "app.oam.dev/requestreconcile": time.Now().String(),
    })
    Expect(k8sClient.Patch(ctx, oCopy.(client.Object), client.Merge)).Should(Succeed())
}
```
Source: `test/e2e-test/suite_test.go:162`

### Ginkgo CLI Flags
```bash
ginkgo -v ./test/e2e-test                         # verbose, all tests
ginkgo -v --focus="PostDispatch" ./test/e2e-test  # run specific test
ginkgo -v -r e2e/application                      # recursive under directory
```

## Test File Naming
| Pattern | Purpose |
|---|---|
| `*_suite_test.go` / `suit_test.go` | Ginkgo bootstrap + envtest setup |
| `*_test.go` (package `foo`) | White-box unit tests (same package) |
| `*_test.go` (package `foo_test`) | Black-box unit/integration tests |
| `test/e2e-test/` | Full E2E against live cluster |
| `test/e2e-multicluster-test/` | Multicluster E2E |
| `test/e2e-addon-test/` | Addon lifecycle E2E |

## YAML Fixtures in Tests
Tests embed YAML as raw string constants directly in `_test.go` files (not separate fixture files):
```go
const workloadDefinition = `
apiVersion: core.oam.dev/v1beta1
kind: WorkloadDefinition
...
`
```
Source: `pkg/controller/core.oam.dev/v1beta1/application/apply_test.go:47`

Some E2E tests read YAML from files via `readAppFromFile(filename)` helper (`test/e2e-test/trait_test.go:36`).

## Logging in Tests
```go
logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))
```
This routes controller-runtime logs to GinkgoWriter so they appear in `ginkgo -v` output.
