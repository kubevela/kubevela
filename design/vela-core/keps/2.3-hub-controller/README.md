# KEP-2.3: Hub Application-Controller Integration

**Status:** Drafting (Not ready for consumption)
**Parent:** [vNext Roadmap](../README.md)

---

## Overview

KEP-2.3 defines the hub application-controller: its responsibilities, the reconcile pipeline, how it parses Applications and resolves Definitions, how it snapshots Definitions and dispatches Component CRs, how it injects traits from policies, and how it watches Component health and manages ordering.

---

## Hub: application-controller

- Resolves `ComponentDefinition` definitions and snapshots them at reconcile time
- Injects traits from policies into Component specs
- Dispatches `Component` CRs to spokes — one resource per component, via the Dispatcher
- Watches `Component.status.isHealthy` for orchestration decisions (dependsOn, workflow gating)
- Manages Application-level concerns: ordering, policies, exports aggregation
- Minimal RBAC — only needs permission to create/watch `Component` CRs on spokes

---

## Architectural Principle

The fundamental architectural insight is that **trying to control everything from the hub is not an effective model**. Workflows, health evaluation, and resource dispatch all benefit from running on the spoke with full in-cluster access. The hub's job is orchestration — not execution.

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

The application-controller's job shrinks dramatically: resolve Definitions, inject traits, dispatch Component CRs, watch health, manage ordering. All execution moves to the spoke.

---

## Hub Reconcile Pipeline

```
1. Resolve ComponentDefinition + snapshot → sign with (name + namespace + revision) salt
2. Resolve dependsOn Component.status.exports → inject into Component spec
3. Inject policy-driven traits into Component spec
4. Dispatcher.Dispatch(Component CR) → spoke
5. Watch Component.status.isHealthy
6. When healthy: read Component.status.exports → pass to dependent Components
```

---

## Hub Dispatcher Interface

The hub dispatcher is dramatically simplified — it only needs to deliver a single `Component` CR.

```go
// Dispatcher delivers a Component CR to a target spoke
type Dispatcher interface {
    Dispatch(ctx context.Context, component *Component) error
    Watch(ctx context.Context, component *Component) (StatusWatcher, error)
    Delete(ctx context.Context, component *Component) error
}

// StatusWatcher watches Component.status.isHealthy on the spoke
type StatusWatcher interface {
    IsHealthy() (bool, error)
    Exports() (map[string]any, error)
}
```

---

## Resolving Definitions and Snapshotting

At reconcile time the hub:

1. Resolves the `ComponentDefinition` referenced by each component entry in the Application
2. Snapshots the resolved Definition — the full CUE package (component template, referenced trait CUE, any referenced WorkflowStepDefinition CUE)
3. Encrypts the snapshot with the per-spoke AES-256-GCM symmetric key (see KEP-2.5)
4. Embeds the encrypted snapshot in `Component.spec.definitionSnapshot.inline`
5. Signs the snapshot with the per-spoke Definition signing key — the signature annotation `component.oam.dev/snapshot-signature` is set on the Component CR

This means the spoke always renders against the Definition that was current at the time the hub last reconciled. Definition upgrades on the hub propagate to spokes on the next reconcile.

---

## Trait Injection from Policies

**Policies inject traits** — policies are purely application-layer concerns; pure functions over application state that attach traits to matching Components before dispatch.

**Policies never reach the spoke** — the Component controller only sees traits, regardless of whether they were user-declared or policy-injected.

The three trait phases:

| Phase | Input | Can do |
|---|---|---|
| `pre` | `context.parameters` | Mutate input properties; override dispatcher |
| `default` | Full context except `context.status` | Patch existing outputs; add new outputs |
| `post` | Full context including `context.status` | Dispatch additional resources after health confirmed |

---

## Dispatching Component CRs

The hub creates one `Component` CR per component in the Application. Each Component CR carries:

- `spec.type` — the ComponentDefinition name
- `spec.properties` — the resolved properties (after `fromParameter` substitution)
- `spec.traits` — the merged trait list (user-declared + policy-injected)
- `spec.dispatcher` — the resolved Dispatcher reference and parameters
- `spec.dependsOn` — the dependency list with resolved imports
- `spec.definitionSnapshot` — the encrypted, signed Definition snapshot

```yaml
apiVersion: core.oam.dev/v2alpha1
kind: Component
metadata:
  name: my-postgres
  namespace: team-a
  annotations:
    component.oam.dev/snapshot-signature: "<sig>"
spec:
  type: postgres-database
  dispatcher:
    name: ocm-prod
    parameters:
      cluster: "prod-cluster-1"
      placement: "prod"
  properties:
    version: "14"
    storage: "10Gi"
    dbName:  "appdb"
  traits:
    - type: apply-once
      properties:
        selector: { resourceTypes: ["StatefulSet"] }
  dependsOn:
    - name: my-config-service
      imports:
        dbPassword: secretValue
  definitionSnapshot:
    revision: 3
    inline: "<AES-256-GCM ciphertext>"
```

---

## Watching Component.status.health

The hub watches `Component.status.isHealthy` via the Dispatcher's `status` CUE transform — which maps the dispatcher's wrapping structure (e.g. ManifestWork status) back to a normalised `componentHealth` boolean.

For OCM the `status` transform maps `ManifestWork.status.resourceStatuses` → `componentHealth`. For cluster-gateway it reads `Component.status.isHealthy` directly via the proxy. Definition authors never need to think about this — it is a Dispatcher concern.

---

## Application.status.placements Model

The hub maintains `Application.status.placements` — a map from component name to the set of clusters the Component was dispatched to, along with per-cluster health status:

```yaml
status:
  placements:
    my-postgres:
      - cluster: prod-cluster-1
        namespace: team-a
        isHealthy: true
    my-cache:
      - cluster: prod-cluster-1
        namespace: team-a
        isHealthy: true
```

This is populated from `Component.status.isHealthy` as read through the Dispatcher's `componentHealth` expression.

---

## fromDependency Hub-Mediated Resolution and Ordering

`fromDependency` wires a component's input properties to an upstream component's exported values. The hub mediates this:

1. The hub dispatches upstream components first (determined by the dependency graph)
2. The hub watches the upstream `Component.status.isHealthy`
3. Once healthy, the hub reads `Component.status.exports` from the upstream Component
4. The hub injects the imported values into the downstream Component's `spec.properties` (replacing the `fromDependency` directives)
5. The hub dispatches the downstream Component

Dependent Components receive their imports in `context.parameters` — transparent to the Definition template.

For OCM, exports are declared as ManifestWork feedback rules on `Component.status.exports.*` fields. The hub reads them from ManifestWork status.

**Schema validation at admission:** The `from*` family is validated at admission time — the hub admission webhook resolves both the consuming component's `parameter{}` schema and the supplying Definition's `exports{}` schema and validates type compatibility between the two. See the cross-KEP section below.

---

## ApplicationRevision Snapshot

When an `ApplicationRevision` is created, the hub resolves and snapshots all Definition definitions the Application was rendered against. The snapshot set in vNext includes:

| Definition type | Purpose |
|---|---|
| `ComponentDefinition` | Component rendering and exports schema |
| `TraitDefinition` | Trait pipeline rendering |
| `WorkflowStepDefinition` | Workflow step execution |
| `PolicyDefinition` | Policy rendering |
| `SourceDefinition` | Source resolution schema and CueX template |

Re-renders and rollbacks of a historical `ApplicationRevision` always use the snapshotted Definition versions — not whatever is currently installed in the cluster. For `SourceDefinition` this is particularly important: the `output{}` schema in the snapshot determines the `ConfigTemplate` version used for cache lookups, ensuring cache correctness across controller restarts and Definition upgrades.

---

## Application Parameters & fromParameter

Applications support an optional `spec.parameters` map — a free-form set of values that represent application-level constants shared across components, traits, policies, and operations. This avoids duplicating cross-cutting values (regions, tier names, cost centres) across every component's properties.

### Templated Applications

When an Application is created from an `ApplicationDefinition`, `spec.parameters` is validated against the Definition's `parameter{}` schema at admission time. The Application controller renders the Application spec from the Definition template with `parameter` values injected — component properties, trait properties, and policy properties are all CUE-rendered, so `parameter.*` references resolve naturally.

### Non-Templated Applications

Non-templated Applications also accept `spec.parameters` as a free-form map. Rather than a CUE render pass, property values may reference parameters via `fromParameter` — an explicit substitution directive that sits inline within `properties`:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
spec:
  parameters:
    region: us-west-2
    tier: production
    enableMultiAz: true

  components:
    - name: my-db
      type: aws-rds
      properties:
        # regular value — no substitution
        instanceClass: db.t3.medium
        # fromParameter resolves before Definition rendering
        region:
          fromParameter: region
        multiAz:
          fromParameter: enableMultiAz

    - name: my-cache
      type: elasticache
      properties:
        region:
          fromParameter: region
        tier:
          fromParameter: tier
```

The Application controller resolves all `fromParameter` references before handing properties to the Definition renderer. If a referenced key is absent from `spec.parameters`, the controller surfaces a validation error at reconcile time with a clear message identifying the missing key.

`fromParameter` is valid within `properties` at any depth — including nested objects and array entries. It is not valid as a map key, only as a value.

### Operations

`spec.parameters` is available in Operations via `context.appParams`. For templated Applications with a known schema, `OperationDefinition` authors may declare `requiredAppDefinition` to rely on specific keys safely. For non-templated Applications, authors should use CUE defaults (`context.appParams.region | "us-east-1"`) or declare dependencies explicitly in the Operation's own `parameter{}` schema.

`fromParameter` is not available in `OperationDefinition` templates — Operations use `context.appParams` directly.

---

## The `from*` Family — Render-Time Resolution & Schema Validation

The `from*` family (`fromParameter`, `fromSource`, `fromDependency`) is fully specified in [KEP-2.21](../2.21-from-resolution/README.md). The hub application-controller is the execution point for the resolution model described there.

The hub's specific responsibilities in this area:

- Executing the single-pass recursive resolution for `fromParameter` and `fromSource` at render time
- Deferring and re-queuing components whose `fromDependency` references are not yet resolvable (upstream not yet healthy)
- Running admission-time schema validation via the hub admission webhook for all three directive types
- Topologically ordering component dispatch based on the `fromDependency` graph

---

## ApplicationDefinition Integration

When an Application references an `ApplicationDefinition` via `spec.definition`, the hub:

1. Resolves the `ApplicationDefinition`
2. Evaluates the CUE template with the provided `spec.parameters`
3. Gets back a fully-formed Application spec
4. Reconciles the resolved spec exactly as if the developer had written it by hand

The Application CR becomes minimal — just a definition reference and parameters:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: my-app
  namespace: team-a
spec:
  definition: web-service
  parameters:
    image:    "myapp:v1.2.0"
    replicas: 3
    domain:   "myapp.example.com"
    dbSize:   "20Gi"
```

> **Cross-KEP note:** `ApplicationDefinition` and `PolicyDefinition` are detailed in [KEP-2.9](../2.9-app-policy-definitions/README.md).

---

## Hub / Spoke Architecture Summary

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

### Why the OCM Problem Is Resolved

OCM wraps a single `Component` CR in a `ManifestWork`. The spoke component-controller does all the work locally. OCM feeds back `isHealthy: true` from `Component.status` — a single boolean field, trivially declared as a feedback rule. No complex status path declarations, no constrained workflow steps, no push proxies.

---

## Cross-KEP References

- **API types** — `Component`, `ComponentDefinition`, `Dispatcher` CRDs — [KEP-2.1](../2.1-core-api/README.md).
- **Spoke reconcile pipeline** — what the spoke does after receiving a Component CR — [KEP-2.2](../2.2-spoke-controller/README.md).
- **Dispatcher implementations** — local, cluster-gateway, OCM — [KEP-2.4](../2.4-dispatchers/README.md).
- **Credential model** — AES-256-GCM snapshot encryption, dispatch integrity HMAC, per-spoke signing keys — [KEP-2.5](../2.5-credential-model/README.md).
- **ApplicationDefinition & PolicyDefinition** — [KEP-2.9](../2.9-app-policy-definitions/README.md).
- **fromSource** — SourceDefinition and lazy render-scoped resolution — [KEP-2.16](../2.16-source-definition/README.md).
- **fromDependency** — Component exports and dependency graph — [KEP-2.17](../2.17-component-exports/README.md).
