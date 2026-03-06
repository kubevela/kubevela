# KubeVela Codebase Conventions

## File & Package Layout

- API types live in `apis/core.oam.dev/v1beta1/` and `apis/core.oam.dev/v1alpha1/`
- Controller logic lives in `pkg/controller/core.oam.dev/v1beta1/`
- Shared schemes, helpers, and utilities live in `pkg/utils/`, `pkg/oam/`, `pkg/oam/util/`
- The global `common.Scheme` used by all tests and fakes is defined in `pkg/utils/common/common.go`
- Each sub-package has its own `*_test.go` files placed alongside source files (white-box) or in `package foo_test` (black-box)

## Go Idioms

### Error Handling
- Always handle errors explicitly; never discard with `_` unless there is a documented `init()` reason
- Standard library errors (`fmt.Errorf`, `errors.New`) are used for simple cases
- `github.com/pkg/errors` (`errors.Wrap`, `errors.Wrapf`) is used for wrapping errors with context in `pkg/resourcekeeper/statekeep.go` and similar files
- Kubernetes API errors are aliased: `kerrors "k8s.io/apimachinery/pkg/errors"` — always check `kerrors.IsNotFound(err)`, never raw string comparisons
- Use `client.IgnoreNotFound(err)` in reconcilers when a missing resource is a no-op (e.g. `pkg/controller/core.oam.dev/v1beta1/core/workflow/workflowstepdefinition/workflowstepdefinition_controller.go:69`)
- Wrap error messages with context using `errors.WithMessage` (crossplane/crossplane-runtime pattern, e.g. `pkg/controller/core.oam.dev/v1beta1/application/application_controller.go:193`)

### Naming
- Reconciler structs are named `Reconciler` and embed `client.Client` directly
- Options structs are named `options` (unexported) and embedded in `Reconciler`
- Constants use `camelCase` for unexported, `UPPER_SNAKE` in Makefile targets, `CamelCase` for exported
- Error message string constants are `errFoo = "cannot foo bar"` (all lowercase, no period)
- Label/annotation keys follow `app.oam.dev/foo` or `oam.dev/foo` convention (see `pkg/oam/` for the label constants)

### Context
- `context.Context` is always the first parameter of any function doing I/O or API calls
- Contexts are enriched with trace metadata via `monitorContext.NewTraceContext` in the main reconcile loop (`application_controller.go:109`)
- Namespace is stored in context via `oamutil.SetNamespaceInCtx(ctx, app.Namespace)`

### Interfaces and Dependency Injection
- Interfaces are used for the `client.Client` dependency — tests pass `fake.NewClientBuilder()` implementations
- The `resourcekeeper` package exposes `ResourceKeeper` interface and returns `*resourceKeeper` (unexported concrete type)
- Mock clients are in `pkg/oam/mock/client.go` — a thin wrapper around `sigs.k8s.io/controller-runtime/pkg/client/fake`

## Controller-Runtime Patterns

### Reconciler Structure
`pkg/controller/core.oam.dev/v1beta1/application/application_controller.go`
```
type Reconciler struct {
    client.Client
    Scheme   *runtime.Scheme
    Recorder event.Recorder
    options
}
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error)
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error
```

### Return Values
- Return `ctrl.Result{}, nil` when done with no re-queue needed
- Return `ctrl.Result{RequeueAfter: duration}, nil` for scheduled re-syncs (not an error)
- Return `ctrl.Result{}, err` only when the reconciler framework should handle retry
- The application controller uses a `r.result(err).ret()` helper that wraps the above logic
- `common.ApplicationReSyncPeriod` is used for the normal periodic re-sync

### Event Watching
- ResourceTracker is watched via `Watches(&v1beta1.ResourceTracker{}, EnqueueRequestsFromMapFunc(findObjectForResourceTracker))` — NOT `.Owns()`, because resources are owned indirectly
- The predicate function on UpdateFunc filters out status-only changes to prevent infinite reconcile loops (`application_controller.go:591-630`)
- `EnableResourceTrackerDeleteOnlyTrigger = true` (package-level var) further restricts ResourceTracker events to deletions only

### Event Filtering — Anti-Infinite-Loop Pattern
```go
WithEventFilter(predicate.Funcs{
    UpdateFunc: func(e ctrlEvent.UpdateEvent) bool {
        // Compare generation to determine if spec changed
        if old.Generation != newApp.Generation { return true }
        // Filter status-only or managedField-only updates
        ...
    },
})
```

### Status Patching
- `r.patchStatus` is used (not `r.Update`) to avoid conflicting writes on the full object
- Conditions use `condition.ReadyCondition("Phase")` and `condition.ErrorCondition("Phase", err)` from `apis/core.oam.dev/condition`

### SetupWithManager
Always wires: `ctrl.NewControllerManagedBy(mgr).Watches(...).WithOptions(...).WithEventFilter(...).For(...).Complete(r)`

## kubebuilder Markers
- RBAC markers placed directly above `Reconcile` functions: `// +kubebuilder:rbac:groups=...,resources=...,verbs=...`
- Scaffold import comment: `// +kubebuilder:scaffold:imports` in suite test files

## Import Aliasing Conventions
| Alias | Package |
|---|---|
| `kerrors` | `k8s.io/apimachinery/pkg/api/errors` |
| `metav1` | `k8s.io/apimachinery/pkg/apis/meta/v1` |
| `corev1` | `k8s.io/api/core/v1` |
| `appsv1` | `k8s.io/api/apps/v1` |
| `ctrl` | `sigs.k8s.io/controller-runtime` |
| `ctrlHandler` | `sigs.k8s.io/controller-runtime/pkg/handler` |
| `ctrlEvent` | `sigs.k8s.io/controller-runtime/pkg/event` |
| `logf` | `sigs.k8s.io/controller-runtime/pkg/log` |

## Global Scheme
`pkg/utils/common/common.go` exports `common.Scheme` — a `*runtime.Scheme` pre-registered with all KubeVela, Kubernetes, Kruise, Terraform, OCM, and Gateway API types. All fake clients in tests use this scheme via `fake.NewClientBuilder().WithScheme(common.Scheme)`.

## CUE Templates
- Defined inline in `v1beta1.TraitDefinitionSpec.Schematic.CUE.Template` as multi-line strings
- `context.output` provides the rendered workload at runtime
- `parameter` provides user inputs
- Use `outputs: foo: { ... }` for PostDispatch traits that produce additional resources
- CUE files in `vela-templates/definitions/` and `pkg/workflow/providers/` are formatted with `cue fmt`

## Code Generation
- After API type changes, run `make generate && make manifests`
- CRDs land in `charts/vela-core/crds/` and are referenced in `envtest.Environment.CRDDirectoryPaths`
