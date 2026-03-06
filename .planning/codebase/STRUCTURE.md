# KubeVela Directory Structure

## Top-level Layout

```
kubevela/
├── cmd/                    — binary entry points
├── apis/                   — CRD Go types (generated)
├── pkg/                    — all library/business logic
├── test/                   — E2E and integration tests
├── charts/vela-core/       — Helm chart for deployment
├── config/crd/             — kustomize CRD patches
├── contribute/             — developer guides
└── design/                 — design docs (ADRs, API references)
```

## cmd/

```
cmd/
├── core/
│   ├── main.go                        — process entry; calls app.NewCoreCommand()
│   ├── main_e2e_test.go               — E2E smoke test wiring
│   └── app/
│       ├── server.go                  — run() orchestrates startup
│       ├── bootstrap.go               — manager creation helpers
│       ├── options/options.go         — CoreOptions struct (all flags)
│       ├── config/
│       │   ├── admission.go           — webhook cert/port flags
│       │   ├── application.go         — app controller flags
│       │   ├── multicluster.go        — cluster-gateway flags
│       │   ├── reconcile.go           — concurrent reconciles, resync period
│       │   ├── sharding.go            — sharding config
│       │   └── feature.go            — feature gate flags
│       └── hooks/
│           └── crdvalidation/         — pre-start CRD compatibility check
└── plugin/
    └── main.go                        — vela CLI plugin entry (separate binary)
```

## apis/

```
apis/
├── apis.go                            — scheme registration entry
├── core.oam.dev/
│   ├── v1beta1/
│   │   ├── application_types.go       — Application, ApplicationSpec, ApplicationStatus
│   │   ├── applicationrevision_types.go
│   │   ├── resourcetracker_types.go   — ResourceTracker CRD type; ResourceTrackerType enum
│   │   ├── componentdefinition_types.go
│   │   ├── definitionrevision_types.go
│   │   ├── core_types.go              — WorkloadDefinition, TraitDefinition
│   │   ├── policy_definition.go
│   │   ├── workflow_step_definition.go
│   │   └── zz_generated.deepcopy.go  — generated
│   ├── v1alpha1/
│   │   ├── policy_types.go            — inline policy types
│   │   ├── applyonce_policy_types.go
│   │   ├── garbagecollect_policy_types.go
│   │   ├── sharedresource_policy_types.go
│   │   ├── takeover_policy_types.go
│   │   ├── readonly_policy_types.go
│   │   ├── resource_update_policy_types.go
│   │   └── envbinding_types.go
│   ├── common/
│   │   └── types.go                   — ApplicationComponent, ApplicationPhase, WorkloadStatus
│   └── condition/
│       └── condition.go               — condition helpers (ReadyCondition, ErrorCondition)
└── types/
    ├── types.go                       — CapType enum, system namespaces, constants
    ├── capability.go                  — Capability struct for definitions
    ├── componentmanifest.go           — ComponentManifest (rendered workload+traits)
    └── multicluster.go               — ControlPlaneClusterVersion, cluster type constants
```

## pkg/ — Core Logic

```
pkg/
├── controller/
│   ├── common/
│   │   ├── vars.go                    — ApplicationReSyncPeriod and other global vars
│   │   └── logs.go
│   ├── core.oam.dev/
│   │   ├── oamruntime_controller.go   — shared reconciler base (Args, Setup pattern)
│   │   └── v1beta1/
│   │       ├── setup.go               — registers all 5 controllers with manager
│   │       ├── application/
│   │       │   ├── application_controller.go  — Reconciler.Reconcile() main loop
│   │       │   ├── apply.go                   — AppHandler struct; dispatch logic
│   │       │   ├── dispatcher.go              — StageType (Pre/Default/Post), DispatchOptions
│   │       │   ├── generator.go               — GenerateApplicationSteps()
│   │       │   ├── workflow.go                — workflow status helpers
│   │       │   ├── revision.go                — AppRevision create/update
│   │       │   ├── assemble/assemble.go       — assembles component manifests
│   │       │   └── application_metrics.go     — Prometheus metrics
│   │       └── core/
│   │           ├── components/componentdefinition/componentdefinition_controller.go
│   │           ├── traits/traitdefinition/traitdefinition_controller.go
│   │           ├── policies/policydefinition/policydefinition_controller.go
│   │           └── workflow/workflowstepdefinition/workflowstepdefinition_controller.go
│   └── utils/
│       ├── capability.go              — definition → Capability conversion
│       └── utils.go                  — controller utility helpers
│
├── appfile/
│   ├── parser.go                      — ApplicationParser.GenerateAppFile(); resolves CUE defs
│   ├── appfile.go                     — AppFile struct; Workload/Trait model
│   ├── template.go                    — Template loading from definition CRs
│   ├── validate.go                    — AppFile validation helpers
│   └── dryrun/
│       ├── dryrun.go                  — dry-run rendering without applying
│       └── diff.go                   — live diff against existing AppRevision
│
├── resourcetracker/
│   ├── app.go                         — CreateRootRT, CreateVersionedRT, ListApplicationRTs
│   ├── tree.go                        — resource tree traversal helpers
│   └── utils.go                      — RT name helpers; RT naming conventions
│
├── resourcekeeper/
│   ├── resourcekeeper.go              — ResourceKeeper interface + resourceKeeper impl
│   ├── dispatch.go                    — Dispatch(): apply resources, update RT ManagedResources
│   ├── delete.go                      — Delete() logic
│   ├── gc.go                          — GarbageCollect(): mark-and-sweep via RT comparison
│   ├── gc_rev.go                      — GC ApplicationRevisions and ComponentRevisions
│   ├── statekeep.go                   — StateKeep(): re-apply drifted resources
│   ├── componentrevision.go           — DispatchComponentRevision / DeleteComponentRevision
│   ├── admission.go                   — policy-based admission checks (shared resources etc.)
│   ├── cache.go                       — resourceCache for RT data within one reconcile
│   └── options.go                    — GCOption, DispatchOption, DeleteOption functional opts
│
├── multicluster/
│   ├── virtual_cluster.go             — VirtualCluster, InitClusterInfo(), NewClusterClient()
│   ├── cluster_management.go          — RegisterCluster, DeregisterCluster, ListClusters
│   ├── cluster_metrics_management.go  — per-cluster metrics collection
│   ├── errors.go                      — cluster-specific error types
│   └── utils.go                      — ContextWithCluster, ContextInLocalCluster helpers
│
├── policy/
│   ├── topology.go                    — TopologyPolicy: compute cluster+namespace placement
│   ├── override.go                    — OverridePolicy: patch component properties
│   ├── replication.go                 — ReplicationPolicy: fan-out to multiple namespaces
│   ├── common.go                      — shared policy parsing helpers
│   └── envbinding/
│       ├── placement.go               — EnvBinding cluster selector logic
│       └── patch.go                  — component patch application
│
├── workflow/
│   ├── workflow.go                    — ConvertWorkflowStatus(); IsFailedAfterRetry()
│   ├── step/
│   │   ├── generator.go               — GenerateApplicationSteps() → WorkflowInstance + runners
│   │   ├── dependency.go              — step dependency resolution
│   │   └── types.go                  — StepGeneratorOption
│   ├── providers/
│   │   ├── oam/apply.go               — built-in `apply-component` provider
│   │   ├── multicluster/
│   │   │   ├── deploy.go              — `deploy` step: multicluster fan-out dispatch
│   │   │   └── multicluster.go       — cluster list/status query provider
│   │   ├── query/                     — resource query provider (VelaQL)
│   │   ├── config/                    — config secret provider
│   │   └── terraform/                 — Terraform resource provider
│   └── template/
│       ├── load.go                    — load step templates from WorkflowStepDefinitions
│       └── static/                   — built-in CUE step templates (apply-component, suspend)
│
├── webhook/
│   └── core.oam.dev/
│       ├── register.go                — registers all webhook handlers with manager
│       └── v1beta1/
│           ├── application/
│           │   ├── mutating_handler.go    — defaulting webhook
│           │   └── validating_handler.go  — admission validation
│           ├── componentdefinition/
│           ├── traitdefinition/
│           ├── policydefinition/
│           └── workflowstepdefinition/
│
├── cue/
│   ├── definition/                    — CUE definition rendering and schema generation
│   └── process/                      — CUE process context for template evaluation
│
├── oam/
│   ├── util/                          — OAM label/annotation helpers, namespace context
│   └── (labels, annotations, finalizer constants)
│
├── auth/                              — RBAC / impersonation for multi-tenant apps
├── features/                          — feature gate definitions
├── cache/                             — informer cache configuration
├── addon/                             — addon registry readers (GitHub, Gitee, OSS, local)
├── monitor/
│   ├── metrics/                       — Prometheus metric definitions
│   └── watcher/                      — application monitor background goroutine
└── utils/
    ├── apply/                         — server-side apply / patch applicator
    ├── common/                        — shared scheme, REST mapper
    └── errors/                        — typed error helpers
```

## test/

```
test/
├── e2e-test/                          — single-cluster E2E (Ginkgo, uses envtest + real cluster)
│   ├── suite_test.go                  — suite bootstrap
│   ├── application_test.go
│   ├── trait_test.go
│   ├── postdispatch_trait_test.go
│   ├── app_resourcetracker_test.go
│   └── testdata/                      — fixture YAMLs
├── e2e-multicluster-test/             — multicluster E2E
│   ├── suite_test.go
│   ├── multicluster_test.go
│   ├── multicluster_standalone_test.go
│   └── testdata/
└── e2e-addon-test/                    — addon install/uninstall E2E
```

## Naming Conventions

- Controller files: `<resource>_controller.go` in package matching the resource group
- Test suites: `suite_test.go` per package (Ginkgo `RunSpecs`)
- Generated files: `zz_generated.deepcopy.go` (never edit manually)
- CUE templates: `*.cue` files in `pkg/workflow/template/static/` and addon directories
- ResourceTracker names:
  - root: `<app-name>-<app-namespace>`
  - versioned: `<app-name>-v<generation>-<app-namespace>`
  - component-revision: `<app-name>-comp-rev-<app-namespace>`
- Definition short names: `cd` (ComponentDefinition), `td` (TraitDefinition), `pd` (PolicyDefinition), `rt` (ResourceTracker)

## Charts

```
charts/vela-core/
├── Chart.yaml
├── templates/
│   ├── kubevela-controller.yaml       — Deployment for vela-core
│   ├── _helpers.tpl
│   ├── addon_registry.yaml
│   ├── admission-webhooks/            — WebhookConfiguration + cert-manager Certificate
│   ├── cluster-gateway/               — ClusterGateway service + cert
│   └── velaql/                        — built-in VelaQL ConfigMap views
└── crds/                             — CRD YAML manifests (generated via make manifests)
```
