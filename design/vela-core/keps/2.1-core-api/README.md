> ⚠️ **Early concept draft.** This KEP is an early-stage exploration. It is **incomplete and may be inaccurate**, its direction is unsettled, and it should not be relied upon for implementation or as a description of committed behaviour. Expect substantial change.

# KEP-2.1: `core.oam.dev/v2alpha1` API Types & CRDs

**Status:** Drafting (Not ready for consumption)
**Parent:** [vNext Roadmap](../README.md)

---

## Overview

KEP-2.1 defines `core.oam.dev/v2alpha1` — a new *version* of KubeVela's existing `core.oam.dev` API group, not a new group. The Definition types are not new resources: `ComponentDefinition`, `TraitDefinition`, and `WorkflowStepDefinition` already exist under `core.oam.dev/v1beta1` and are carried into `v2alpha1` with **structural changes** (the unified `outputs` list, directory/blueprint authoring, annotation-driven behaviour). One primitive is genuinely added at this version: `Component`, elevated to a first-class instance CR. This KEP establishes the `v2alpha1` structure and the conventions shared across the Definition types.

> **API versioning note:** v2 uses `core.oam.dev/v2alpha1` — the same API group as KubeVela v1 (`core.oam.dev`), but a new version. This is intentional: it enables coexistence of v1.x and v2.x types in the same cluster without group conflicts, and simplifies migration tooling since both generations share a common group. The v1.x `Application` and related types continue to live under `core.oam.dev/v1beta1`; the new v2 primitives are introduced under `core.oam.dev/v2alpha1`.

---

## API Group & Resources

**API Version:** `core.oam.dev/v2alpha1`

| Resource | Scope | Runs on | Role |
|---|---|---|---|
| `ComponentDefinition` | Cluster | Hub + Spoke | Template: CUE package + schema |
| `TraitDefinition` | Cluster | Hub + Spoke | Template: named trait with phase |
| `WorkflowStepDefinition` | Cluster | Spoke | Template: reusable workflow step |
| `Component` | Namespace | Spoke (dispatched by hub) | Instance of a ComponentDefinition |

---

## What Changes in v2alpha1

### Definition types: carried forward, restructured

`ComponentDefinition`, `TraitDefinition`, `WorkflowStepDefinition`, `PolicyDefinition`, and `ApplicationDefinition` are all CUE-authored, and all use the same input-schema identifier: **`parameter`** (singular), read at evaluation time as **`context.parameter`**. They differ only in packaging shape, not in that convention:

- **Blueprint shape** (`ComponentDefinition`): a multi-file blueprint referenced via `spec.source` (git/OCI) or `spec.inline`. Input schema lives in `variables.cue` as top-level `parameter`; resources live in `outputs.cue`. See the `ComponentDefinition` example below.
- **Inline-template shape** (`TraitDefinition`, `WorkflowStepDefinition`, and the simpler definition types): a `spec.template:` CUE block with `parameter` and `output` inside it. See the `TraitDefinition`/`WorkflowStepDefinition` examples below.

In both shapes the schema field is `parameter` (singular); it is never pluralised.

### Component is a First-Class Instance CR

`Component` can be deployed standalone without an Application. It manages its own full lifecycle on the spoke.

### `outputs: []` Replaces `output:`/`outputs:`

The new unified `outputs` field is a named list with `type` (defaults to `resource`), `value`, and `statusPaths` per entry. It supports `type: component` for nested Component instances.

**Output types:**
- `resource` (default, can be omitted) — a Kubernetes object, applied via SSA on the spoke
- `component` — a nested `Component` instance, created on the spoke, managed independently

### Annotations as Behaviour Contract

`component.oam.dev/apply-strategy`, `component.oam.dev/ownership`, `component.oam.dev/gc-strategy` replace feature flags, policy-driven apply options, and per-manifest cluster routing labels.

### Definition Authoring as Directories

Like Terraform modules: `metadata.cue` is the required entrypoint, all other files merged via CUE unification; supports inline and OCI source modes.

---

## Definition Authoring: Directory Structure

A `ComponentDefinition` is authored as a directory of CUE files, merged via CUE unification before evaluation. This is analogous to Terraform modules — the directory is the unit of authorship, reuse, and versioning.

```
postgres-database/
  metadata.cue      # required — identifies the directory as a Definition package
  variables.cue     # input schema (OpenAPI v3)
  outputs.cue       # output resources
  exports.cue       # exported values for dependent Components
  workflow.cue      # lifecycle steps (create, upgrade, delete)
  health.cue        # isHealthy expression
```

`metadata.cue` is the required entrypoint. All other files are optional and merged. The controller (and CLI tooling) loads and unifies all `.cue` files in the directory before evaluation.

The `ComponentDefinition` CRD supports two source modes:
- **Directory reference** — points to a git repo path or OCI artifact (primary authoring mode)
- **Inline** — embeds merged CUE directly in the CRD spec (for simple cases or generated/distributed form)

### Example: metadata.cue

```cue
metadata: {
  name: "postgres-database"
}
```

### Example: variables.cue

```cue
// Input schema — validated against Component.spec.properties at admission time
parameter: {
  version: string
  storage: string
  dbName:  string
}
```

### Example: outputs.cue

```cue
// outputs is a list; controller indexes by name for context.status resolution
// type defaults to "resource" — most users never need to specify it
outputs: [
  {
    name: "database"
    // type omitted — defaults to "resource"
    value: {
      apiVersion: "apps/v1"
      kind:       "StatefulSet"
      metadata: name: context.name
      spec: template: spec: containers: [{
        image: "postgres:\(context.parameter.version)"
      }]
    }
  },
  {
    name: "service"
    value: {
      apiVersion: "v1"
      kind:       "Service"
      metadata: name: context.name
    }
  },
  {
    name: "cache"
    type: component       // nested Component instance — full lifecycle managed independently
    value: {
      type: "redis"
      properties: { maxMemory: "256mb" }
    }
  },
]
```

### Example: health.cue

```cue
// Evaluated against live resources on the spoke — full in-cluster access, no feedback rules needed
isHealthy:
  context.status.database.readyReplicas == context.status.database.replicas &&
  context.status.database.replicas > 0
```

### Example: exports.cue

```cue
// Evaluated after isHealthy is true
// Available to dependent Components via dependsOn.imports
exports: {
  connectionString: "postgresql://\(context.status.service.clusterIP):5432/\(context.parameter.dbName)"
  host:             context.status.service.clusterIP
  port:             5432
}
```

---

## ComponentDefinition CRD

```yaml
apiVersion: core.oam.dev/v2alpha1
kind: ComponentDefinition
metadata:
  name: postgres-database
spec:
  # Option 1: directory reference (git or OCI)
  source:
    oci: "registry.example.com/blueprints/postgres-database:v1.2.0"

  # Option 2: inline merged CUE (generated or simple cases)
  inline: |
    outputs: [...]
    workflow: { ... }
    isHealthy: ...
```

---

## Component CRD

Namespace-scoped. In standalone mode, created directly by the user on the spoke. In application-driven mode, dispatched by the hub application-controller.

```yaml
apiVersion: core.oam.dev/v2alpha1
kind: Component
metadata:
  name: my-postgres
  namespace: team-a
  annotations:
    component.oam.dev/snapshot-signature: "<sig>"  # hub-signed; spoke verifies before executing
spec:
  type: postgres-database         # resolves ComponentDefinition
  properties:
    version: "14"
    storage: "10Gi"
    dbName:  "appdb"
  traits:                         # resolved and injected by hub before dispatch
    - type: apply-once
      properties:
        selector: { resourceTypes: ["StatefulSet"] }
  dependsOn:
    - name: my-config-service
      imports:
        dbPassword: secretValue

  # Present only when dispatched by hub application-controller
  # Absent in standalone mode — component-controller loads Definition from local cluster
  definitionSnapshot:
    revision: 3                   # included in dispatch integrity HMAC
    inline: "<AES-256-GCM ciphertext of Definition CUE, encrypted with per-spoke symmetric key>"

status:
  observedGeneration: 3
  isHealthy: true
  exports:
    connectionString: "postgresql://10.0.0.5:5432/appdb"
    host: "10.0.0.5"
    port: 5432
  conditions: [...]
```

---

## Definition Snapshot Security

When the hub dispatches a `Component` CR with a `definitionSnapshot`, the snapshot is encrypted using the per-spoke AES-256-GCM symmetric key established at spoke registration (KEP-2.5). The `inline` field contains ciphertext — not plaintext CUE. A user with write access to Component CRs on the spoke cannot inject arbitrary CUE — they do not hold the symmetric key required to produce valid ciphertext, and any tampering causes decryption to fail.

Definitions are stored on the hub only. The spoke has no access to the hub API and does not fetch definitions independently — `spec.definitionSnapshot.inline` is the sole source of truth for rendering.

### Encryption

The hub encrypts the full Definition snapshot (Component CUE template, trait CUE, and any referenced WorkflowStepDefinition CUE) with the **per-spoke AES-256-GCM symmetric key**:

```
ciphertext = AES-256-GCM-Encrypt(definitionCUE, perSpokeSymmetricKey)
```

AES-256-GCM provides both confidentiality and authenticated integrity in a single operation — any modification to the ciphertext causes decryption to fail. The dispatch integrity HMAC annotation (KEP-2.5) additionally covers `definitionSnapshot.revision`, binding the revision into the per-Component signature so it cannot be tampered with independently.

### Decryption & Rendering

The spoke component-controller:
1. Decrypts `spec.definitionSnapshot.inline` using its copy of the per-spoke AES-256-GCM symmetric key
2. If decryption succeeds — uses the decrypted Definition CUE for rendering
3. If decryption fails — rejects the Component, emits a warning event, does not render

### Standalone Mode

Standalone Components (no `definitionSnapshot`) do not use this mechanism — the component-controller loads `ComponentDefinition` resources from the local spoke cluster. Definition execution is gated by RBAC on `ComponentDefinition` resources on that cluster.

---

## TraitDefinition CRD

```yaml
apiVersion: core.oam.dev/v2alpha1
kind: TraitDefinition
metadata:
  name: apply-once
spec:
  phase: default                   # pre | default | post  (default if omitted)
  template: |
    patch: outputs: [ for o in context.outputs if list.Contains(parameter.selector.resourceTypes, o.value.kind) {
      name: o.name
      metadata: annotations: "component.oam.dev/apply-strategy": "once"
    }]
```

---

## WorkflowStepDefinition CRD

Spoke-side only. Executed by the component-controller with full in-cluster access.

```yaml
apiVersion: core.oam.dev/v2alpha1
kind: WorkflowStepDefinition
metadata:
  name: register-service
spec:
  template: |
    #do: "http-request"
    method: "POST"
    url:     "http://\(parameter.registry)/register"
    body: {
      name:     context.name
      endpoint: context.status.service.clusterIP
    }
```

---

## CUE Context

The same `context` object is available across all CUE evaluation phases. On the spoke, all fields are available without constraint — the component-controller has full in-cluster access.

```cue
context: {
  name:       string
  namespace:  string
  parameter: { ... }        // input properties; mutated by pre traits; imports injected here

  // available during default trait phase and later:
  outputs: [...]            // rendered output list

  // available after dispatch and health check:
  status: {
    [outputName]: {
      [fieldName]: _        // resolved from live resources on spoke
    }
  }
}
```

---

## Behaviour Annotations

Traits set well-known `component.oam.dev/` annotations on output resources. The spoke component-controller reads these when applying resources. (Note: these annotation keys retain the `component.oam.dev/` prefix as stable, well-known annotation names; they are not API group references.)

| Annotation | Values | Effect |
|---|---|---|
| `component.oam.dev/apply-strategy` | `once`, `always` | Control re-apply behaviour |
| `component.oam.dev/ownership` | `read-only`, `take-over`, `shared` | Resource ownership semantics |
| `component.oam.dev/gc-strategy` | `retain`, `delete` | Garbage collection on Component deletion |

Note: `component.oam.dev/force-local` is no longer needed — the component-controller always runs on the spoke and always has local access.

---

## Status & Health

### Spoke-side: context.status

The spoke component-controller reads live resources directly from the spoke API server. `statusPaths` declared on each output define which fields to extract and surface into `context.status` for use in `isHealthy`, `exports`, and workflow steps:

```cue
{
  name: "database"
  value: { ... }
  statusPaths: [
    { name: "readyReplicas", path: ".status.readyReplicas" },
    { name: "replicas",      path: ".status.replicas" },
  ]
}
```

The spoke controller does a direct `client.Get` on each output resource and extracts the declared paths into `context.status.database.readyReplicas` etc.

### isHealthy

Evaluated on the spoke against `context.status`:

```cue
isHealthy:
  context.status.database.readyReplicas == context.status.database.replicas &&
  context.status.database.replicas > 0
```

Written to `Component.status.isHealthy`, which the hub reads to track Component health.

---

## Exports & Dependencies

`exports` are evaluated on the spoke after `isHealthy` is true, written to `Component.status.exports`.

The hub reads `Component.status.exports` and injects imported values into dependent Component specs before dispatching them. Dependent Components receive their imports in `context.parameter` — transparent to the Definition template.

For OCM, exports are declared as ManifestWork feedback rules on `Component.status.exports.*` fields. The hub reads them from ManifestWork status.

---

## Upgrade & Drift

- **Drift correction** — the spoke component-controller continuously reconciles diverged resources via SSA
- **Re-render** — triggered when `Component.spec` changes or `definitionSnapshot` hash changes
- **Rollback** — revert `Component.spec` on the hub; hub re-dispatches updated Component CR to spoke
- **Revision tracking** — `Component.status.observedGeneration` tracks the last successfully reconciled generation

No `ComponentDefinitionRevision` CRD needed. Definition versioning is managed at the source level (git, OCI) and pinned via `ComponentDefinition.spec.source`.

---

## Cross-KEP References

- **`from*` family** (`fromParameter`, `fromSource`, `fromDependency`) — render-time value resolution directives that operate inline within `properties`. These are cross-KEP concerns spanning KEP-2.3 (hub), KEP-2.16 (`fromSource`), and KEP-2.17 (`fromDependency`). Not duplicated here; see those KEPs.
- **Spoke reconcile pipeline** — the ordered execution sequence for the spoke component-controller is defined in [KEP-2.2](../2.2-spoke-controller/README.md).
- **Hub reconcile pipeline** — how the hub resolves Definitions, snapshots, injects traits, and dispatches is defined in [KEP-2.3](../2.3-hub-controller/README.md).
- **Credential model** — the AES-256-GCM symmetric key, HMAC dispatch integrity, and per-spoke signing key are defined in [KEP-2.5](../2.5-credential-model/README.md).
- **Dispatcher implementations** — local, cluster-gateway, and OCM Dispatcher types are detailed in [KEP-2.4](../2.4-dispatchers/README.md).
