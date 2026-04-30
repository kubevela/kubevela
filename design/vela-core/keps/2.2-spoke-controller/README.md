# KEP-2.2: Spoke Component-Controller & Workflow Engine

**Status:** Drafting (Not ready for consumption)
**Parent:** [vNext Roadmap](../README.md)

---

## Overview

KEP-2.2 defines the spoke component-controller: the controller that runs on every target cluster, executes CUE rendering, the trait pipeline, lifecycle workflows, health evaluation, and ResourceTracker management. The workflow engine is embedded directly in the component-controller — it is not a separate addon.

---

## Spoke: component-controller

- Runs on every target cluster (installed as part of spoke setup)
- Loads `ComponentDefinition` definitions locally (standalone mode) or from hub-dispatched snapshots (application-driven mode)
- Executes CUE rendering, trait pipeline, and lifecycle workflows
- Has full in-cluster access — no proxy, no feedback rules, no constrained execution
- Manages `ResourceTracker`, GC, health evaluation
- Reflects `Component.status.isHealthy` back to hub

---

## Why This Resolves the OCM Problem

OCM wraps a single `Component` CR in a `ManifestWork`. The spoke component-controller does all the work locally. OCM feeds back `isHealthy: true` from `Component.status` — a single boolean field, trivially declared as a feedback rule. No complex status path declarations, no constrained workflow steps, no push proxies.

```
Hub application-controller
  └─ dispatches Component CR (via OCM ManifestWork / cluster-gateway)
       └─ Spoke component-controller
            ├─ renders CUE template
            ├─ executes workflow (full in-cluster access)
            ├─ manages ResourceTracker + GC
            └─ reflects Component.status.isHealthy
  └─ watches Component.status.isHealthy
  └─ manages dependsOn ordering
```

---

## Spoke Reconcile Pipeline

```
1. Verify definitionSnapshot signature (if present)
2. Load Definition (from snapshot or local cluster)
3. Diff current spec against previous generation → populate:
   - context.event (create / upgrade / delete)
   - context.paramsChanged (bool convenience)
   - context.params.current / context.params.target (full parameter maps)
   - context.apiVersion.current / context.apiVersion.target (from definitionSnapshot or live Definition)
   - context.definitionChanged, context.traitsChanged
4. Load context.workflow from Component.status.workflow
5. If lifecycle event (create/upgrade/delete): execute pre-dispatch workflow steps
   (context.parameters known; resources not yet rendered or applied)
6. Resolve from* directives (fromParameter, fromSource, fromDependency)
7. Run pre traits → mutated parameters
8. Evaluate merged CUE (template + default traits) → outputs list
   (context.workflow available to renderer)
9. dispatch step → SSA-apply outputs to spoke cluster
10. Execute post-dispatch workflow steps (context.outputs available)
11. Persist context.workflow → Component.status.workflow
12. Evaluate isHealthy → write to Component.status.isHealthy
13. If healthy: evaluate exports → write to Component.status.exports
14. Run post traits
```

---

## CUE Rendering Pipeline

The spoke evaluates the merged CUE package (all `.cue` files from the Definition directory unified together). The `context` object is the entry point — it carries input parameters, dispatcher metadata, and previously persisted workflow state.

The rendering pass produces the `outputs` list. Each output entry has a `name`, an optional `type` (defaults to `resource`), a `value` (the Kubernetes object), and optional `statusPaths` for health field extraction.

The renderer always has access to `context.workflow.*` — populated from `Component.status.workflow` before rendering begins, regardless of whether the current reconcile triggered a workflow execution.

---

## Trait Pipeline

Traits are resolved and injected by the hub before dispatch. The spoke component-controller executes them.

```
context.parameters
   └─ [pre traits]              # mutate context.parameters before template evaluates
        └─ CUE evaluation       # Definition template + default traits (CUE unification)
             └─ context.outputs (rendered output list)
                  └─ workflow execution (spoke-side, full in-cluster access)
                       └─ health check (isHealthy — direct resource reads on spoke)
                            └─ exports evaluated → Component.status.exports
                                 └─ [post traits]
```

### Phase Behaviours

| Phase | Input | Can do |
|---|---|---|
| `pre` | `context.parameters` | Mutate input properties; override dispatcher |
| `default` | Full context except `context.status` | Patch existing outputs; add new outputs |
| `post` | Full context including `context.status` | Dispatch additional resources after health confirmed |

---

## Workflow Engine as Embedded Library

The workflow engine is a Go library embedded directly in the component-controller. It is not a separate controller, not an optional addon — it is part of the spoke installation. If a component-controller is running, the workflow engine is available. Definition authors can always rely on workflow steps.

This means:
- `WorkflowStepDefinition` is a guaranteed primitive — no capability detection required
- `create`, `upgrade`, `delete` are first-class lifecycle concepts in the engine, not generic workflow runs
- The engine has native access to Component context, outputs, status, and exports — it is purpose-built for the component lifecycle

---

## Lifecycle Workflows

### The `workflow:` Block

The `workflow.cue` file in a ComponentDefinition declares two lifecycle blocks: `create` and `delete`.

`create` is a single unified lifecycle workflow that runs on every create or upgrade event. `context.event` and change flags let the author branch on what triggered the reconcile. `delete` remains separate — it has fundamentally different semantics and runs pre-render.

```cue
workflow: {
  // Unified create/upgrade workflow — runs on create and upgrade events
  // Steps before dispatch are pre-render; steps after have access to context.outputs
  create: [
    if context.event == "create" {
      { type: "run-job", properties: {
          image: "migrate:latest"
          command: ["./migrate", "up"]
        }
      }
    },
    if context.event == "upgrade" && context.paramsChanged {
      { type: "run-job", properties: {
          image: "migrate:latest"
          command: ["./migrate", "up"]
        }
      }
    },
    { type: "dispatch" },   // ← render + resource apply happens here
    { type: "wait-healthy", selector: { names: ["database"] } },
  ]
  // delete runs pre-render with context.previousParams available
  delete: [
    { type: "run-job", properties: {
        image: "migrate:latest"
        command: ["./migrate", "down"]
      }
    },
    { type: "dispatch-delete" },
  ]
}
```

If `workflow.cue` is absent the component-controller falls back to implicit dispatch — resources are applied immediately after rendering, preserving v1 behaviour.

Since the component-controller runs on the spoke with full in-cluster access, workflow steps execute natively — reading resources, calling in-cluster APIs, checking health — without any dispatcher constraints. `run-job` is reserved for cases that genuinely require a container (migration scripts, tooling only available as a Docker image). All other spoke interactions are native workflow step operations.

### Lifecycle Events

There are two workflow blocks:

- **`create`** — runs on every `create` or `upgrade` event. A single unified block; the author branches on `context.event` and change flags. Steps before `dispatch` are pre-render (parameters known, resources not yet applied). Steps after `dispatch` are post-render (`context.outputs` available).
- **`delete`** — runs on deletion. Always pre-render; `context.previousParams` holds the last known parameter values.

If no event has occurred (reconcile triggered by health check or external drift), no workflow runs — the controller evaluates health and applies StateKeep only.

### Workflow Context

The following context fields are available in workflow steps:

| Field | Available | Description |
|---|---|---|
| `context.event` | Always | `create`, `upgrade`, `delete` |
| `context.paramsChanged` | `create` | Convenience bool — true if any parameter value changed from previous generation |
| `context.definitionChanged` | `create` | Definition revision changed |
| `context.traitsChanged` | `create` | Trait set or trait properties changed |
| `context.params.current` | `create`, `delete` | Full parameter map from the previous generation — for fine-grained diffing |
| `context.params.target` | `create` | Full parameter map being applied this reconcile |
| `context.apiVersion.current` | `create`, `delete` | `apiVersion` of the ComponentDefinition currently running on the cluster |
| `context.apiVersion.target` | `create` | `apiVersion` of the ComponentDefinition being applied this reconcile |
| `context.outputs` | Post-`dispatch` in `create` only | Rendered resource objects after apply; not available in `delete` |
| `context.workflow` | Always | Persisted state map from previous workflow runs (see below) |
| `context.steps.<name>` | After named step completes | Output values from a completed workflow step |

`context.paramsChanged` is a convenience shorthand for `context.params.current != context.params.target`. For simple gates use `context.paramsChanged`; for branching on specific field changes use `context.params.current.<field>` vs `context.params.target.<field>` directly.

`context.apiVersion.current` vs `context.apiVersion.target` enables Definition authors to detect API version migrations and run schema migration steps before upgrading. When both are equal no version change is in progress.

```cue
// Example: API version migration + targeted param diffing
create: [
  if context.apiVersion.current != context.apiVersion.target {
    { type: "helm-migrate-schema", properties: {
        fromVersion: context.apiVersion.current
        toVersion:   context.apiVersion.target
        fromParams:  context.params.current
        toParams:    context.params.target
      }
    }
  }
  if context.event == "upgrade" && context.params.current.version != context.params.target.version {
    { type: "helm-upgrade", properties: { ... } }
  }
  { type: "dispatch" }
]
```

---

## Workflow State Persistence

`context.workflow.*` is a user-facing key/value namespace persisted across reconciles. Workflow steps write to it via `set-workflow-state`; the component-controller persists it to `Component.status.workflow` after each workflow run and injects it into the CUE render context on every subsequent reconcile — whether or not the workflow re-executes.

**This is a component-controller concern, not the workflow engine.** The workflow engine (`kubevela/workflow`) has its own internal context persistence (ConfigMap-backed `ContextBackend`) for step execution state, backoff timers, and suspend state. That is engine-internal plumbing and is not exposed to CUE templates. `context.workflow.*` is a separate, user-facing layer owned by the component-controller:

```
Workflow executes
  → set-workflow-state step writes key/value pairs to step output
  → component-controller collects output after workflow completes
  → writes to Component.status.workflow
  → cleared on next reconcile trigger from status, injected as context.workflow.*
  → renderer and subsequent workflow runs have access to context.workflow.*
```

Between lifecycle events — when no workflow executes — the component-controller loads `Component.status.workflow` directly and injects it as `context.workflow.*` without running any workflow steps. The renderer always has access to the last persisted state.

```cue
// workflow.cue — helm-chart ComponentDefinition
create: [
  if context.event == "create" {
    { type: "helm-install", properties: {
        chart:   context.parameters.chart
        version: context.parameters.version
      }
    }
  }
  if context.event == "upgrade" && context.paramsChanged {
    { type: "helm-upgrade", properties: {
        chart:   context.parameters.chart
        version: context.parameters.version
      }
    }
  }
  // component-controller collects this output and writes to Component.status.workflow
  { type: "set-workflow-state", properties: {
      releaseName: context.steps["helm-install"].releaseName
    }
  }
  { type: "dispatch" }
]
```

```cue
// template.cue — renderer consumes persisted workflow state
// context.workflow.* is always available, populated from Component.status.workflow
output: {
  metadata: annotations: {
    "helm.sh/release-name": context.workflow.releaseName
  }
}
```

**Execution contract:**
- Workflow executes **once per lifecycle event** (create, upgrade, delete) — not on every reconcile
- Between events: no workflow execution, `context.workflow.*` loaded from status
- Definition authors are responsible for **idempotency** of their custom steps — the controller guarantees at-least-once execution per event on transient failures
- The end consumer never sees any of this — they declare component type and properties only

The end consumer never sees any of this — they simply declare the component type and properties. The Definition author encapsulates all workflow and state logic internally.

---

## `set-workflow-state` Built-in Step

`set-workflow-state` writes key/value pairs to `Component.status.workflow`, making them available as `context.workflow.*` on subsequent reconciles and in the renderer.

```cue
{ type: "set-workflow-state", properties: {
    releaseName: context.name
  }
  inputs: [
    { from: "helmResources", parameterKey: "resources" }
  ]
}
```

---

## Selective Step Execution

The workflow author is responsible for deciding which steps run under which conditions — the controller does not attempt to infer this. The `context.event` and change flags (`context.paramsChanged`, `context.definitionChanged`, `context.traitsChanged`) give the author full visibility into what triggered the reconcile, and CUE `if` expressions gate steps accordingly.

```cue
create: [
  // always runs — cheap data refresh, no side effects
  { type: "refresh-catalog-metadata" }
  { type: "set-workflow-state", properties: {
      catalogData: context.steps["refresh-catalog-metadata"].result
    }
  }

  // only runs when properties actually changed — has side effects
  if context.paramsChanged {
    { type: "helm-upgrade" }
  }

  { type: "dispatch" }
]
```

The `dispatch` step itself is idempotent — the controller compares the rendered output hash against what is currently applied on the cluster and skips API calls if nothing has changed. Definition authors do not need to guard `dispatch` — unnecessary applies are prevented automatically regardless of how often the workflow runs.

The key design principle: **workflow steps with side effects should be explicitly gated by the author; `dispatch` does not need to be**.

---

## Built-in Step Types

| Step | Effect |
|---|---|
| `dispatch` | Renders outputs and SSA-applies them to the spoke cluster. Acts as the pre/post render boundary in `create`. |
| `dispatch-delete` | Tears down all ResourceTracker-owned resources. Acts as the pre/post delete boundary in `delete` — steps before it are pre-delete (resources still exist), steps after it are post-delete (resources gone). |
| `wait-healthy` | Waits for specified outputs to reach healthy state before proceeding. |
| `set-workflow-state` | Writes key/value pairs to `Component.status.workflow`, available as `context.workflow.*` on subsequent reconciles and in the renderer. |
| `run-job` | Creates a Job on the spoke, waits for completion. Reserved for cases requiring a container (migration scripts, tooling only available as a Docker image). |

**Custom steps** resolve a `WorkflowStepDefinition` by name. All steps execute spoke-side with full in-cluster access.

---

## Helm via WorkflowStepDefinition

Since workflow steps run natively on the spoke, Helm can be invoked via SDK rather than shelling out to a container:

```cue
// WorkflowStepDefinition: helm-install
template: |
  parameters: {
    chart:   string
    version: string
  }
  #do: "helm-install"
  release:   context.name
  chart:     parameters.chart
  version:   parameters.version
  namespace: context.namespace
```

```cue
// workflow.cue — helm-chart ComponentDefinition
workflow: {
  create: [
    // API version migration — run schema migration before upgrade if version changed
    if context.apiVersion.current != context.apiVersion.target {
      { type: "helm-migrate-schema", properties: {
          fromVersion: context.apiVersion.current
          toVersion:   context.apiVersion.target
          fromParams:  context.params.current
          toParams:    context.params.target
        }
      }
    }
    if context.event == "create" {
      {
        type: "helm-install"
        properties: { ... }
        outputs: [
          { name: "helmResources", valueFrom: "resources" }
        ]
      }
    }
    if context.event == "upgrade" && context.paramsChanged {
      {
        type: "helm-upgrade"
        properties: { ... }
        outputs: [
          { name: "helmResources", valueFrom: "resources" }
        ]
      }
    }
    {
      type: "set-workflow-state"
      properties: { releaseName: context.name }
      inputs: [
        { from: "helmResources", parameterKey: "resources" }
      ]
    }
    { type: "dispatch" }
  ]
  delete: [
    // pre-delete — resources still exist on cluster
    { type: "helm-uninstall", properties: { ... } }
    { type: "dispatch-delete" }   // ← KubeVela deletes resources here
    // post-delete — resources gone; context.params.current and context.workflow still available
    { type: "notify", properties: { message: "teardown complete" } }
  ]
}
```

Context available in `delete` workflow:

| Phase | Available context |
|---|---|
| Pre-`dispatch-delete` | `context.params.current`, `context.workflow`, `context.event` |
| Post-`dispatch-delete` | `context.params.current`, `context.workflow` — `context.outputs` is not available (resources are gone) |

---

## Health Evaluation

Health is evaluated on the spoke against `context.status` — direct resource reads, no feedback rules needed.

`isHealthy` is written to `Component.status.isHealthy`. The hub reads this via the Dispatcher's `componentHealth` expression.

See [KEP-2.1](../2.1-core-api/README.md) for the `isHealthy` expression format and `statusPaths` configuration.

---

## ResourceTracker on Spoke

The spoke component-controller maintains a `ResourceTracker` for each Component — tracking all spoke-side owned resources for GC lifecycle.

```go
// ResourceTracker tracks spoke-side owned resources for GC lifecycle
type ResourceTracker interface {
    Record(ctx context.Context, outputs []*unstructured.Unstructured) error
    GarbageCollect(ctx context.Context) error
    ContainsAll(ctx context.Context, outputs []*unstructured.Unstructured) (bool, error)
}
```

When a Component is deleted or its outputs change, the ResourceTracker drives garbage collection — removing resources no longer declared in the current outputs list.

---

## Deployment Summary

| Component | Standalone | KubeVela bundle |
|---|---|---|
| component-controller + workflow engine | ✓ (spoke installation) | ✓ |
| application-controller | — | ✓ |
| WorkflowRun controller | ✓ (optional) | ✓ (bundled, default on) |

---

## Cross-KEP References

- **API types** — `Component`, `ComponentDefinition`, `TraitDefinition`, `WorkflowStepDefinition` CRDs are defined in [KEP-2.1](../2.1-core-api/README.md).
- **Hub reconcile pipeline** — the hub's role in resolving Definitions, injecting traits, and dispatching Component CRs is in [KEP-2.3](../2.3-hub-controller/README.md).
- **Dispatcher** — how the hub wraps and delivers Component CRs; how health is fed back — [KEP-2.4](../2.4-dispatchers/README.md).
- **Credential model** — definitionSnapshot encryption, dispatch integrity HMAC — [KEP-2.5](../2.5-credential-model/README.md).
- **WorkflowRun controller** — standalone WorkflowRun bundling (separate from the embedded workflow engine) — [KEP-2.7](../2.7-workflowrun/README.md).
