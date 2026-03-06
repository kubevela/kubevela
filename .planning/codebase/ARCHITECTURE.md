# KubeVela Architecture

## Pattern: controller-runtime

KubeVela is built on `sigs.k8s.io/controller-runtime` (kubebuilder-style). The binary is a single `vela-core` process that hosts a `ctrl.Manager` embedding all reconcilers, webhook servers, and health endpoints.

Entry point: `cmd/core/main.go` → `cmd/core/app/server.go:run()`

Startup sequence in `cmd/core/app/server.go`:
1. `syncConfigurations` — copy CLI flags to global vars
2. `setupMultiCluster` — init cluster-gateway if enabled
3. `createControllerManager` — `ctrl.NewManager()`
4. `setupControllers` — register all reconcilers + webhooks
5. `manager.Start(ctx)`

## Layers

```
cmd/core/app/           ← process bootstrap, flag parsing, manager creation
  config/               ← typed flag structs (admission, multicluster, reconcile, sharding…)
  options/              ← CoreOptions aggregating all config structs

pkg/controller/core.oam.dev/v1beta1/setup.go  ← registers all controllers
  application/          ← primary Application reconciler
  core/components/      ← ComponentDefinition controller
  core/traits/          ← TraitDefinition controller
  core/policies/        ← PolicyDefinition controller
  core/workflow/        ← WorkflowStepDefinition controller

pkg/appfile/            ← parses Application spec into internal AppFile/Workload model
pkg/resourcekeeper/     ← ResourceKeeper interface: Dispatch / Delete / GC / StateKeep
pkg/resourcetracker/    ← CRUD helpers for ResourceTracker CRs
pkg/multicluster/       ← VirtualCluster abstraction, cluster-gateway client setup
pkg/policy/             ← topology, override, replication, envbinding logic
pkg/workflow/           ← wraps github.com/kubevela/workflow executor + step generators
pkg/webhook/            ← mutating + validating handlers for Application and Definitions

apis/core.oam.dev/v1beta1/  ← Application, ApplicationRevision, ResourceTracker types
apis/core.oam.dev/v1alpha1/ ← Policy types (ApplyOnce, GC, SharedResource, TakeOver…)
apis/core.oam.dev/common/   ← ApplicationComponent, ApplicationPhase, condition helpers
```

## Application Reconcile Data Flow

File: `pkg/controller/core.oam.dev/v1beta1/application/application_controller.go:106`

```
Reconcile(req)
  │
  ├─ Get Application CR
  ├─ handleFinalizers          — add/remove oam.FinalizerResourceTracker
  ├─ appParser.GenerateAppFile — pkg/appfile/parser.go: resolve CUE templates → AppFile
  ├─ handler.PrepareCurrentAppRevision
  ├─ handler.FinalizeAndApplyAppRevision
  ├─ handler.ApplyPolicies     — pkg/policy/: topology/override/replication
  ├─ handler.GenerateApplicationSteps — pkg/workflow/step/generator.go
  ├─ workflowExecutor.ExecuteRunners  — github.com/kubevela/workflow
  │     └─ each step calls resourceKeeper.Dispatch()
  ├─ applyPostDispatchTraits   — feature-gated; dispatches PostDispatch-stage traits
  │     requires workload to be DispatchHealthy before running
  ├─ evalStatus                — health evaluation → ApplicationRunning | Unhealthy
  ├─ stateKeep                 — resourceKeeper.StateKeep() — drift prevention
  └─ gcResourceTrackers        — resourceKeeper.GarbageCollect() + RT GC
```

## ResourceTracker Pattern

Instead of `.Owns()` (owner references), KubeVela uses cluster-scoped `ResourceTracker` CRs to track every resource an Application manages. This is required because Applications are namespace-scoped but managed resources may span namespaces and clusters.

API type: `apis/core.oam.dev/v1beta1/resourcetracker_types.go`

Three RT types per Application (naming in `pkg/resourcetracker/app.go`):
- **root** RT (`<app>-<ns>`) — resources alive for full app lifetime; cleaned up only on app deletion
- **versioned** RT (`<app>-v<generation>-<ns>`) — resources for a specific generation; GC'd when generation is superseded
- **component-revision** RT (`<app>-comp-rev-<ns>`) — tracks ControllerRevisions for component history

`ResourceKeeper` interface (`pkg/resourcekeeper/resourcekeeper.go:39`):
```go
Dispatch(ctx, resources, applyOpts, ...DispatchOption) error
Delete(ctx, resources, ...DeleteOption) error
GarbageCollect(ctx, ...GCOption) (bool, []ManagedResource, error)
StateKeep(ctx) error
```

Implementation holds `_rootRT`, `_currentRT`, `_historyRTs`, `_crRT` lazily loaded.

Finalizer `resourcetracker.core.oam.dev/finalizer` is set on each RT; deletion walks all RTs and removes managed resources before removing the RT itself.

## Dispatch Stages (PostDispatch Traits)

File: `pkg/controller/core.oam.dev/v1beta1/application/dispatcher.go:64`

Traits execute in three stages:
- `PreDispatch (0)` — before workload is applied
- `DefaultDispatch (1)` — same pass as workload
- `PostDispatch (2)` — after workload is DispatchHealthy (has runtime status: replicas, observedGeneration)

PostDispatch traits are gated by feature flag `features.MultiStageComponentApply`. They are applied in `applyPostDispatchTraits()` once the workflow step has completed and the workload health is evaluated as DispatchHealthy.

Component health progression: `Unhealthy → DispatchHealthy → Healthy`

## Multicluster

Files: `pkg/multicluster/virtual_cluster.go`, `pkg/multicluster/cluster_management.go`

- Uses `github.com/oam-dev/cluster-gateway` as the transport layer
- `VirtualCluster` struct unifies cluster secrets and OCM ManagedCluster implementations
- `InitClusterInfo()` called at startup; bootstraps version info for all registered clusters
- `ClusterLocalName` constant identifies the hub/control-plane cluster
- Multicluster-aware `client.Client` is injected by `setupMultiCluster` in server.go; all resourcekeeper calls route through it
- `pkg/policy/topology.go` + `pkg/policy/envbinding/placement.go` compute which clusters/namespaces a component is placed into
- Workflow step `deploy` (`pkg/workflow/providers/multicluster/deploy.go`) drives fan-out dispatch to remote clusters

## Webhook Layer

Registration: `pkg/webhook/core.oam.dev/register.go`

- `pkg/webhook/core.oam.dev/v1beta1/application/mutating_handler.go` — sets defaults, injects publish version annotation
- `pkg/webhook/core.oam.dev/v1beta1/application/validating_handler.go` — validates component types, policy names, workflow refs
- Definition webhooks validate CUE templates for TraitDefinition, ComponentDefinition, PolicyDefinition, WorkflowStepDefinition

## Sharding

Controlled by `github.com/kubevela/pkg/controller/sharding`. When `EnableSharding=true`, the controller only processes Applications whose shard annotation matches `ShardID`. Configured via `cmd/core/app/config/sharding.go`.

## Key Feature Gates (`pkg/features/`)

- `MultiStageComponentApply` — enables PreDispatch/PostDispatch trait stages
- `ApplyOnce` — disables StateKeep (drift detection)
- `DisableBootstrapClusterInfo` — skips cluster version bootstrap
- `EnableSuspendOnFailure` — pause workflow on step failure
