# KEP-2.15: OperationDefinition & Operation

**Status:** Drafting (Not ready for consumption)
**Parent:** [vNext Roadmap](../README.md)

Day 2 operations are a first-class OAM primitive. `OperationDefinition` and `Operation` allow component and application authors to ship operational runbooks — backup, restore, rotate-credentials, scale-event — alongside their Definitions, with full access to the OAM context of the target Component or Application. KubeVela does not execute the actual work; it orchestrates the context and delegates to external tools (Argo Workflows, Crossplane claims, external APIs) via `WorkflowStepDefinition` primitives.

## OperationDefinition

`OperationDefinition` is a namespaced CRD in the `core.oam.dev/v2alpha1` API version, following the standard Definition authoring model. It is authored alongside the `ComponentDefinition` it serves — a `ComponentDefinition` author publishes zero or more `OperationDefinition`s describing how to operate their component.

The `template:` block contains a `parameter{}` schema and a named map of workflow phase definitions. One phase is designated the `entrypoint`. Each phase is a sequence of `WorkflowStepDefinition` references. Phase transitions are declared via annotations on the phase:

```cue
// backup-operation/template.cue
template: {
  // entrypoint declares which phase runs first
  entrypoint: "preflight"

  phases: {
    preflight: {
      steps: [
        {
          type:       "check-component-health"
          properties: { component: context.componentName }
          // enabled defaults to true. If false, step is skipped.
          // Any CUE expression with access to context is valid.
          enabled: context.status.phase != "deleting"
          // onSkip controls phase transition when a step is skipped.
          // Defaults to onSuccess if omitted.
          onSkip: "backup"
        },
        {
          type:       "notify"
          properties: { message: "Starting backup for \(context.componentName)" }
          // enabled defaults to true — omitting it is equivalent to enabled: true
        },
      ]
      onSuccess: "backup"
      onFailure: "abort"
    }

    backup: {
      steps: [
        {
          type: "argo-workflow"
          properties: {
            workflowTemplate: "s3-backup"
            parameters: {
              bucket:    context.parameters.bucket
              targetKey: "\(context.appName)/\(context.componentName)/\(context.operationName)"
            }
          }
          // Skip the argo workflow step on non-primary clusters;
          // onSkip not set so defaults to onSuccess → "complete"
          enabled: context.cluster.labels["tier"] == "primary"
        },
      ]
      onSuccess: "complete"
      onFailure: "cleanup"
    }

    cleanup: {
      steps: [
        {
          type: "write-status"
          properties: {
            target: { kind: context.scope, name: context.componentName }
            patch:  { lastBackup: { status: "failed", time: context.startTime } }
          }
        },
      ]
      onSuccess: "abort"
    }

    complete: {
      steps: [
        {
          type: "write-status"
          properties: {
            target: { kind: context.scope, name: context.componentName }
            patch:  { lastBackup: { status: "success", time: context.startTime } }
          }
        },
      ]
    }

    abort: {}
  }

  parameter: {
    bucket: string
  }
}
```

## Step-Level enabled and onSkip

Every step in a phase supports two optional fields:

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | CUE bool expression | `true` | Evaluated with full context before the step runs. If `false`, the step is skipped entirely — no execution, no error. |
| `onSkip` | phase name | same as `onSuccess` | Phase to transition to when this step is skipped. If omitted, skipping is treated identically to success. |

This allows definition authors to write a single `OperationDefinition` that adapts to different cluster topologies, component states, or parameter combinations — rather than requiring separate definitions per scenario:

```cue
// Skip an expensive verification step if the operator flags it
{
  type:    "verify-backup-integrity"
  enabled: !context.parameters.skipVerification
  // onSkip not set → defaults to onSuccess
}

// Only send alerts on production clusters; skip silently elsewhere
{
  type:    "pagerduty-alert"
  enabled: context.cluster.labels["env"] == "production"
  onSkip:  "complete"
}

// Skip cleanup if the component is already in a terminal state
{
  type:    "drain-connections"
  enabled: context.status.phase != "failed"
  onSkip:  "abort"
}
```

The `onSkip` → `onSuccess` default is deliberate: a skipped step means the work wasn't needed, not that it failed. Definition authors only need to set `onSkip` explicitly when a skip should route to a different phase than success.

The `scope` field on the `OperationDefinition` declares whether it targets a `Component` or an `Application`:

```yaml
apiVersion: core.oam.dev/v2alpha1
kind: OperationDefinition
metadata:
  name: s3-backup
spec:
  # scope: Component | Application
  # Component: context includes component + application details
  # Application: context includes full application spec; can compose other Operations
  scope: Component
  # allowedWorkloadTypes limits which ComponentDefinition types this Operation can target.
  # Empty means unrestricted.
  allowedWorkloadTypes:
    - aws-s3-bucket
```

The optional `scope: Application` mode provides `context.application` (full Application spec) and allows an Operation to fan out by composing child `Operation` CRs against individual components.

## Cluster Targeting

An Operation may need to run against only a subset of the clusters an Application has dispatched to. Two complementary mechanisms control this:

**1. Explicit cluster selection on the Operation CR** — the operator specifies exactly which clusters to target at invocation time:

```yaml
spec:
  definition: s3-backup
  componentRef:
    name: payments-db
  # clusters restricts execution to the named clusters.
  # If omitted, runs on all clusters the referenced Component is dispatched to.
  clusters:
    - eu-west-1
    - eu-central-1
  parameters:
    bucket: my-backups
```

**2. Definition-author `enabled` expression** — the `OperationDefinition` template declares a CUE `enabled` expression evaluated per-cluster before dispatch. If it returns `false` for a cluster, the operation is skipped on that cluster without error:

```cue
template: {
  // enabled is evaluated once per candidate cluster with context.cluster populated.
  // If false, the operation is silently skipped for that cluster.
  // If omitted, the operation runs on all targeted clusters.
  enabled: context.cluster.labels["region"] == "eu-west-1" ||
            context.cluster.labels["tier"] == "primary"

  entrypoint: "backup"
  phases: { ... }
  parameter: { bucket: string }
}
```

The two mechanisms compose: `spec.clusters` narrows the candidate set first; `enabled` is then evaluated against each candidate. A cluster must pass both to receive the operation. `context.cluster` is added to the CUE context for each candidate cluster, providing its labels, annotations, and name.

## CUE Context

The operation-controller populates the following context before evaluating any phase:

| Field | Source |
|---|---|
| `context.operationName` | `Operation` CR name |
| `context.componentName` | `spec.componentRef.name` |
| `context.appName` | owning Application name (from Component labels) |
| `context.appLabels` | owning Application labels |
| `context.appAnnotations` | owning Application annotations |
| `context.namespace` | Operation CR namespace |
| `context.parameters` | `Operation.spec.parameters` rendered against `OperationDefinition.parameter` schema |
| `context.scope` | `Component` or `Application` |
| `context.status` | current `Component.status` or `Application.status` at operation start time |
| `context.startTime` | ISO8601 timestamp when the Operation was triggered |
| `context.cluster` | name, labels, and annotations of the target cluster (from Cluster KEP) |
| `context.appParams` | `Application.spec.parameters` — see Application Parameters contract below |
| `context.application` | full Application spec (only when `scope: Application`) |

## Application Parameters Contract

`Application.spec.parameters` is an optional free-form map available to Operations via `context.appParams`. The safety contract depends on whether the Application was created from an `ApplicationDefinition`:

**Templated Applications** (`spec.definition` is set): the `ApplicationDefinition` declares a validated parameter schema. An `OperationDefinition` may declare `requiredAppDefinition` to assert it only runs against Applications of that type. When set, the operation-controller enforces this at admission time and `context.appParams` is schema-guaranteed — the `OperationDefinition` can freely rely on specific keys:

```cue
// metadata.cue
operationDefinition: {
  name: "dr-failover"
  // Enforced at admission — Operation CR is rejected if the target
  // Application was not created from this ApplicationDefinition.
  requiredAppDefinition: "multi-region-app"
}

// template.cue — safe to rely on appParams keys; schema is guaranteed
template: {
  phases: {
    failover: {
      steps: [
        { type: "dispatch-operation", properties: {
            definition: "spoke-failover"
            parameters: {
              // guaranteed present by multi-region-app parameter schema
              primaryRegion:   context.appParams.primaryRegion
              secondaryRegion: context.appParams.secondaryRegion
            }
          }
        }
      ]
      onSuccess: "complete"
      onFailure: "rollback"
    }
  }
  parameter: {}
}
```

**Non-templated Applications** (`spec.definition` is not set): `context.appParams` is populated from whatever `spec.parameters` the Application declares, with no schema guarantee. `OperationDefinition` authors must handle missing keys defensively using CUE defaults or conditional expressions. This is the author's responsibility — the operation-controller provides no safety net for non-templated apps.

```cue
// Defensive usage for non-templated apps
primaryRegion: context.appParams.primaryRegion | "us-east-1"
```

The rule: **if your `OperationDefinition` needs to rely on `context.appParams`, declare `requiredAppDefinition`**. If you can't — because the operation is designed for generic use across any Application — declare the values you need in `parameter{}` and have the caller wire them explicitly from `spec.parameters`.

## Operation CR

`Operation` is a namespace-scoped, run-to-completion CR. Each `Operation` CR represents a single execution. Recurring operations are achieved by creating new `Operation` CRs (via CronJob, automation, or `vela operation run`).

```yaml
apiVersion: core.oam.dev/v2alpha1
kind: Operation
metadata:
  name: backup-payments-db-20260328
  namespace: payments-prod
spec:
  definition: s3-backup
  # componentRef targets a Component in the same namespace.
  # For scope: Application, use applicationRef instead.
  componentRef:
    name: payments-db
  # clusters optionally restricts which clusters the operation runs on.
  # If omitted, targets all clusters the Component is dispatched to,
  # subject to the Definition's enabled expression.
  clusters:
    - eu-west-1
  parameters:
    bucket: my-backups
  # execution declares where the operation-controller itself runs.
  # hub: hub operation-controller executes (useful for cross-cluster coordination).
  # spoke: dispatched to each target cluster (default).
  execution: spoke
```

## Dispatch

When `execution: spoke` (the default), the operation-controller dispatches one `Operation` CR per target cluster using the same Dispatcher mechanism as `Component` dispatch. The spoke's operation-controller executes the phase workflow locally with full in-cluster access. When `execution: hub`, the hub's operation-controller executes it directly — useful for operations that coordinate across clusters or interact with hub-side resources.

## Status Writeback

The `write-status` `WorkflowStepDefinition` is provided by the operation-controller runtime. It accepts a `target` (the Component or Application CR reference) and a `patch` (a CUE-expressed partial status object) and applies it as a strategic merge patch to `status.operationStatus` on the target. This allows component authors to surface operational state — last backup time, last restore result, credential rotation timestamp — directly on the Component status without requiring custom controllers.

```cue
// write-status step usage
{ type: "write-status", properties: {
    target: { kind: "Component", name: context.componentName }
    patch: {
      lastBackup: {
        status:    "success"
        time:      context.startTime
        operation: context.operationName
      }
    }
  }
}
```

## WorkflowStepDefinition Scope

The existing `scope` field on `WorkflowStepDefinition` is used to restrict which execution contexts a step may be used in:

```yaml
# scope controls where a WorkflowStepDefinition can be referenced
scope:
  - Application   # usable in Application workflow steps
  - Operation     # usable in OperationDefinition phases
  - WorkflowRun   # usable in standalone WorkflowRun
```

Steps without a `scope` field are unrestricted. This prevents operational steps (e.g. `write-status`, `argo-workflow`) from being accidentally referenced in application delivery workflows where they don't belong.

## Relationship to Other KEPs

- **KEP-2.2 (Spoke component-controller)** — the spoke operation-controller shares the workflow engine embedded in the component-controller; bundled in the same binary
- **KEP-2.4 (Dispatcher)** — `execution: spoke` uses the same Dispatcher lookup as Component dispatch
- **KEP-2.7 (WorkflowRun controller)** — `WorkflowStepDefinition` primitives are shared across Operations and WorkflowRuns
