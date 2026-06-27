# KEP: vNext Roadmap — Umbrella Design

**Status:** Drafting (Not ready for consumption)
**Date:** 2026-03-26
**Authors:** TBD

> **vNext** is the codename for KubeVela's next major architectural programme. These KEPs collectively define vNext, which will culminate in a KubeVela 2.0 release. Individual features may land in 1.x releases in advance of that milestone — the 2.0 release marks the point at which the full set is complete and supported.

> This is an umbrella KEP capturing the full vNext Roadmap design. It will be decomposed into focused sub-KEPs for implementation. Sub-KEPs planned:
> - `KEP-2.1`: `core.oam.dev/v2alpha1` API types & CRDs (`ComponentDefinition`, `TraitDefinition`, `WorkflowStepDefinition`, `Component`, `Dispatcher`)
> - `KEP-2.2`: Spoke component-controller & workflow engine
> - `KEP-2.3`: Hub application-controller integration
> - `KEP-2.4`: Dispatcher implementations (local, cluster-gateway, OCM)
> - `KEP-2.5`: Credential model & token rotation
> - `KEP-2.6`: KubeVela Operator (`KubeVela` CR)
> - `KEP-2.7`: WorkflowRun controller bundling
> - `KEP-2.8`: KubeVela v1 → v2 migration tooling
> - `KEP-2.9`: `ApplicationDefinition` & `PolicyDefinition`
> - `KEP-2.10`: Compositions — `type: component` outputs compose smaller Definitions into higher-order abstractions
> - `KEP-2.11`: Definition testing framework
> - `KEP-2.12`: `ObservabilityDefinition` — fleet-level observability views across Component instances
> - `KEP-2.13`: Declarative Addon Lifecycle — GitOps-compatible Addon CR continuous reconciliation, drift correction, addon-of-addons composition
> - `KEP-2.14`: `TenantDefinition` & `Tenant` — first-class multi-tenancy; namespace + RBAC + quota + cluster access provisioning via CUE-authored tenancy archetypes
> - `KEP-2.15`: `OperationDefinition` & `Operation` — Day 2 operations as OAM-context-aware orchestration primitives; phased workflow execution delegating to external tools via WorkflowStepDefinitions
> - `KEP-2.16`: `SourceDefinition` & `fromSource` — declarative external data resolution; lazy, render-scoped, cacheable via ConfigTemplate/Config
> - `KEP-2.17`: Component Exports & same-topology `fromDependency` — contract-driven intra-app data binding; acyclic dependency graph; no workflow steps required
> - `KEP-2.18`: `ConfigTemplate` & `Config` CRDs — promote ConfigMap-backed config primitives to first-class CRDs with admission validation, native GitOps ergonomics, and proper RBAC
> - `KEP-2.19`: Cross-topology `fromDependency` — named topology groups; hub-mediated resolution across clusters; depends on KEP-2.4 and KEP-2.18
> - `KEP-2.20`: Module & API Line Versioning — module identity model, definition naming convention, API line coexistence and deprecation lifecycle; extends KEP-2.13
> - `KEP-2.21`: `from*` family resolution model — render-time resolution pass, admission schema validation, and shorthand syntax shared by `fromParameter`, `fromSource`, and `fromDependency`
> - `KEP-2.22`: Multi-Instance Addons — `instance` field in `_module.cue`, per-instance Addon CR naming, namespace-scoped definition isolation

---

## High-Level Overview

The vNext Roadmap is an ambitious architectural evolution across nine major areas:

### 1. New API Model (`core.oam.dev/v2alpha1`)
- **Definition types replace Definitions** — `ComponentDefinition`, `TraitDefinition`, `WorkflowStepDefinition`, `PolicyDefinition`, `ApplicationDefinition` follow a consistent CUE authoring model (metadata block + `template:` at top level, `parameter` and `output` inside `template`)
- **`Component` is a first-class instance CR** — can be deployed standalone without an Application
- **`SourceDefinition` & `fromSource`** (KEP-2.16) — platform engineers publish reusable external data resolvers (HTTP/APIs, ConfigMaps, Secrets, cluster metadata); application authors consume inline via `fromSource` in properties; lazy render-scoped resolution with TTL caching persisted via ConfigTemplate/Config; `fromContext` superseded by purpose-built `SourceDefinition` definitions
- **Component Exports & `fromDependency`** (KEP-2.17) — `ComponentDefinition` authors declare an explicit `exports` contract; application authors wire component outputs to downstream inputs via `fromDependency`; implicit ordering dependency graph, cycle detection at admission, no workflow steps required
- **`spec.parameters` on Application** — application-level constants shared across components, traits, policies, and operations; templated apps validated against `ApplicationDefinition` schema; non-templated apps use free-form map with `fromParameter` for inline property substitution; available in Operations as `context.appParams`
- **`outputs: []` replaces `output:`/`outputs:`** — named list with `type` (defaults to `resource`), `value`, and `statusPaths` per entry; supports `type: component` for nested instances
- **`Dispatcher` is CUE-parameterised** — `dispatch:` and `status:` CUE transforms declare how to deliver and how to retrieve status; schema declared inline in CUE consistent with all Definition types
- **Annotations as behaviour contract** — `component.oam.dev/apply-strategy`, `component.oam.dev/ownership`, `component.oam.dev/gc-strategy` replace feature flags, policy-driven apply options, and per-manifest cluster routing labels
- **Definition authoring as directories** — like Terraform modules; `metadata.cue` is the required entrypoint, all other files merged via CUE unification; supports inline and OCI source modes

### 2. Hub / Spoke Architecture
- **Hub (application-controller)** — resolves Definitions, snapshots at reconcile time, injects traits from policies, dispatches `Component` CRs, watches health. Minimal RBAC — only needs permission to create/watch `Component` CRs.
- **Spoke (component-controller)** — runs on every target cluster, executes CUE rendering, trait pipeline, workflows, health evaluation. Full in-cluster access. No OCM constraints.
- **OCM drastically simplified** — hub dispatches a single `Component` CR via ManifestWork; spoke does all the work; hub watches one feedback field (`isHealthy`) — no complex statusPaths or feedback rules
- **Workflow engine embedded** in component-controller — always present, no optional addon required; `WorkflowStepDefinition` is a guaranteed primitive

### 3. Trait & Policy Model
- **Traits own behaviour** — anything that modifies a component's operational behaviour is a trait (`apply-once`, `gc`, `read-only`, `ownership`)
- **Three phases** — `pre` (mutate inputs before CUE evaluation), `default` (CUE unification with template), `post` (dispatched after health check)
- **Policies inject traits** — purely application-layer concerns; pure functions over application state that attach traits to matching Components before dispatch
- **Policies never reach the spoke** — Component controller only sees traits, regardless of whether they were user-declared or policy-injected

### 4. Dispatcher Model
- **Two-phase lookup** — local namespace → `vela-system`; `ComponentDefinition.defaultDispatcher` as final fallback
- **`dispatch:` CUE transform** — wraps `context.outputs` into dispatcher-specific format (e.g. ManifestWork); parameters schema declared inline
- **`status:` CUE transform** — maps dispatcher's wrapping structure back to normalised `context.outputs[x].status` and `componentHealth` boolean
- **Three built-in types** — `local` (SSA direct), `cluster-gateway` (proxy), `ocm` (ManifestWork)
- **`force-local` annotation removed** — spoke always has local access; no longer needed

### 5. Workflow & Lifecycle
- **`create` / `delete`** — first-class lifecycle concepts; `create` is a unified workflow covering both create and upgrade events, branching on `context.event` and change flags (`context.paramsChanged`, `context.definitionChanged`); `dispatch` step within the workflow controls when resources are applied, acting as the pre/post render boundary
- **`context.workflow.*` persistence** — workflow steps write to `context.workflow`, persisted to `Component.status.workflow` and available to the renderer on subsequent reconciles; Definition authors encapsulate workflow complexity internally, consumers see only component type and properties
- **Built-in steps** — `dispatch`, `dispatch-delete`, `wait-healthy`, `run-job`
- **`run-job` narrowly scoped** — reserved for genuine container requirements (migrations, external tooling); all other spoke operations are native workflow steps with full in-cluster access
- **Helm via `WorkflowStepDefinition`** — SDK invocation spoke-side, not container shelling; works correctly with OCM pull-based setups
- **Standalone `WorkflowRun` controller** — bundled in KubeVela Helm chart by default; eliminates current pain point where nothing can presume its presence

### 6. Security & Credential Model
- **No long-lived static tokens** — all credentials short-lived and rotated automatically
- **Bi-directional trust** — spoke controls its own identity (hub cannot mint spoke tokens); hub issues unique per-spoke Definition signing keys (one compromise doesn't affect other spokes)
- **Three-factor token refresh** — requires (1) spoke cluster write access + (2) hub per-spoke signing key (signs the request Secret) + (3) secure channel; compromising any two is insufficient
- **Spoke-initiated secure channel** — spoke drives all credential refreshes; hub is purely reactive; no hub-held spoke kubeconfigs after bootstrap
- **Kubernetes Events for audit** — `SpokeRegistered`, `ComponentDispatched`, `SigningKeyRotated`, `WorkflowStarted/Completed/Failed`, `ComponentHealthy/Degraded` etc.; standard log pipelines scrape automatically

### 7. Platform Engineering
- **`ApplicationDefinition`** — app platform engineers compose platform primitives into opinionated developer-facing constructs; developers consume via `definition:` + `parameters:` or hand-craft their own Applications from available building blocks
- **`PolicyDefinition`** — named, reusable policy definitions; referenced by name in `ApplicationDefinition` and `Application` CRs
- **`OperationDefinition`** (KEP-2.15) — component authors ship Day 2 runbooks (backup, restore, rotate-credentials) alongside their Definitions; phases of `WorkflowStepDefinition` steps execute with full OAM context; delegates actual work to external tools (Argo Workflows, Crossplane, external APIs); `write-status` step surfaces operational state back onto Component/Application status
- **`TenantDefinition`** (KEP-2.14) — platform engineers define tenancy archetypes (team, environment, partner); declares namespaces, RBAC, quotas, cluster access label selectors, and an Application or ApplicationDefinition for shared tenant infrastructure; `Tenant` CRs instantiate a type with parameters and pin to a `DefinitionRevision`
- **`ObservabilityDefinition`** — fleet-level observability views across Component instances; scoped `selector` + `fields` projection keeps queries cheap; multiple teams can define independent views over the same or multiple Definition types; outputs metrics (Prometheus), logs (structured stdout), alerts (Kubernetes Events)
- **Compositions** (KEP-2.10) — `type: component` outputs compose smaller Definitions into higher-order abstractions; dependency-ordered rollout, health aggregation, parameter surface reduction
- **Definition testing framework** (KEP-2.11) — CUE-native unit testing for all Definition types; `tests/` directory alongside Definition CUE files; `vela definition test` CLI command

### 8. KubeVela Operator
- **`KubeVela` CR** — single resource describing the entire installation; `role: hub | spoke | standalone` with admission webhook enforcing role-specific field constraints
- **Operator-managed** — installs, configures, upgrades with dependency ordering, and drift-corrects the KubeVela installation; drives spoke registration automatically on `role: spoke`
- **Feature flags and security settings** — `features:`, `security:` blocks replace scattered Helm values and environment variables
- **Addon references** — `addons:` lists addon names; `Addon` CR (separate KEP) handles lifecycle
- **`KubeVela.status`** — surfaces health of all controllers and addons in one place

### 9. Existing Capabilities Updated for vNext
- Self-service catalog, dry-run/preview, schema validation at authoring time — exist in v1, updated for new API model
- Definition marketplace — exists but out of date; revived with OCI-backed registry
- Multi-tenancy, compliance guardrails, cost visibility — solved by `Component` as first-class CR; Kyverno/OPA work naturally

---

## Summary

The vNext Roadmap is an ambitious evolution of the KubeVela platform. It introduces a new API version `core.oam.dev/v2alpha1` that extracts and rewrites KubeVela's component rendering and dispatch machinery into a standalone, reusable system with a clean hub/spoke architecture.

> **API versioning note:** v2 uses `core.oam.dev/v2alpha1` — the same API group as KubeVela v1 (`core.oam.dev`), but a new version. This is intentional: it enables coexistence of v1.x and v2.x types in the same cluster without group conflicts, and simplifies migration tooling since both generations share a common group. The v1.x `Application` and related types continue to live under `core.oam.dev/v1beta1`; the new v2 primitives are introduced under `core.oam.dev/v2alpha1`.

The core primitives are:
- `ComponentDefinition` — a CUE-templated, schema-validated definition of a composable Kubernetes component, authored as a directory of CUE files
- `TraitDefinition` — a named, reusable trait with a declared execution phase (pre/default/post)
- `WorkflowStepDefinition` — a reusable workflow step type executed spoke-side with full in-cluster access
- `Component` — a standalone instance of a ComponentDefinition that manages its own full lifecycle on the spoke
- `Dispatcher` — a CUE-parameterised delivery mechanism that abstracts how Component CRs are delivered to spokes and how status is retrieved
- `ApplicationDefinition` — a CUE-parameterised composition of components, traits, and policies into an opinionated application construct
- `PolicyDefinition` — a named, reusable policy definition that ApplicationDefinitions and Applications reference by name
- `KubeVela` operator CR — a single resource that describes and reconciles the entire KubeVela installation for a given cluster role

This system is built incrementally on KubeVela v1 foundations, extracting and evolving the component rendering and dispatch stack. KubeVela's Application becomes a thin orchestration layer over `core.oam.dev/v2alpha1` primitives rather than owning the rendering and dispatch stack.

This is a **v2.0 breaking change**. It does not attempt backwards compatibility with KubeVela v1 APIs.

---

## Motivation

KubeVela's component rendering and dispatch is tightly coupled to `Application` reconciliation. This creates several problems:

- Components cannot be instantiated without the overhead of an Application, workflow, and policy
- Dispatch topology is controlled via policies, which attach to components indirectly through workflow steps — creating tangled, hard-to-reason-about behaviour
- Operational modifiers (apply-once, garbage collection, read-only) are scattered across policies, feature flags, and apply options rather than being explicit component concerns
- The render/dispatch pipeline has accumulated parallel code paths, implicit feature flags, and cluster routing embedded as labels on manifests
- Definition authoring is constrained to monolithic CUE strings embedded in YAML
- Hub-side orchestration of spoke-side operations creates fundamental tension with pull-based dispatch models (OCM)

The goal is a system where:
- A `Component` can be deployed standalone, without an Application
- Each component owns its dispatch mechanism
- Operational behaviour is controlled by traits, set explicitly or injected by policies
- The rendering pipeline is a clean, testable, single-path orchestrator
- Definitions are authored as directories of CUE files, like Terraform modules
- Hub and spoke responsibilities are cleanly separated

### Alignment with OAM

This design is directionally consistent with the Open Application Model spec — components, traits, and application-level policies — but consciously diverges from the letter of the spec in two ways:

1. `Component` becomes a namespace-scoped instance resource rather than a cluster-scoped definition
2. Policies are purely application-layer concerns that inject traits into components, rather than first-class OAM primitives

OAM Scopes are not addressed in this proposal. Topology (placement) is handled via the `Dispatcher` reference on each Component, which policies can override.

---

## Architecture: Hub / Spoke Split

The fundamental architectural insight is that **trying to control everything from the hub is not an effective model**. Workflows, health evaluation, and resource dispatch all benefit from running on the spoke with full in-cluster access. The hub's job is orchestration — not execution.

### Hub: application-controller

- Resolves `ComponentDefinition` definitions and snapshots them at reconcile time
- Injects traits from policies into Component specs
- Dispatches `Component` CRs to spokes — one resource per component, via the Dispatcher
- Watches `Component.status.isHealthy` for orchestration decisions (dependsOn, workflow gating)
- Manages Application-level concerns: ordering, policies, exports aggregation
- Minimal RBAC — only needs permission to create/watch `Component` CRs on spokes

### Spoke: component-controller

- Runs on every target cluster (installed as part of spoke setup)
- Loads `ComponentDefinition` definitions locally (standalone mode) or from hub-dispatched snapshots (application-driven mode)
- Executes CUE rendering, trait pipeline, and lifecycle workflows
- Has full in-cluster access — no proxy, no feedback rules, no constrained execution
- Manages `ResourceTracker`, GC, health evaluation
- Reflects `Component.status.isHealthy` back to hub

### Why this resolves the OCM problem

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

## The `from*` Family — Render-Time Resolution & Schema Validation

`fromParameter`, `fromSource`, and `fromDependency` form the complete `from*` family of value-level substitution directives. All three operate inline within `properties` at any depth, are detected structurally (not by string matching), and resolve to concrete values before the Definition renderer sees the properties map.

### Render-Time Resolution Model

The Application controller performs a single recursive pass over the merged `properties` map for each component immediately before handing the properties to the Definition renderer. The traversal:

1. Walks every node in the properties tree (objects, arrays, scalar leaves)
2. When it encounters a node whose sole key is `fromParameter`, `fromSource`, or `fromDependency`, it replaces the entire node with the resolved value
3. Passes the fully-resolved properties to the CUE Definition renderer

`fromDependency` nodes are only resolvable once the referenced component is healthy and has written its `exports` to `Component.status.exports`. Until then, the consuming component is not rendered — the controller defers it and re-queues when the upstream export becomes available (see KEP-2.17). `fromSource` nodes are resolved by evaluating the `SourceDefinition` CueX template (with caching, see KEP-2.16). `fromParameter` nodes are resolved immediately from `spec.parameters`.

The recursive pass is deterministic and single-pass for `fromParameter` and `fromSource`. It is topologically ordered for `fromDependency` — the dependency graph determines evaluation order.

### Schema Contract Validation at Admission Time

The `from*` directives are deliberately declarative rather than opaque. This means the *schema* of every substitution reference can be validated at `kubectl apply` time — before reconciliation, before rendering, and before any runtime values exist. Three validations run at admission:

| Directive | Schema source | Validated at admission |
|---|---|---|
| `fromParameter` | `spec.parameters` (free-form) or `ApplicationDefinition.parameter{}` schema | Key existence (free-form) or key type match (templated) |
| `fromSource` | `SourceDefinition.output{}` schema | Source name exists in `spec.sources`; `path` refers to a declared output field; output field type is compatible with the consuming component's `parameter{}` schema at the given property path |
| `fromDependency` | `ComponentDefinition.exports{}` schema | Dependency component exists in the Application; `path` refers to a declared `exports` field; exports field type is compatible with the consuming component's `parameter{}` schema at the given property path |

The key insight: even though `fromSource` and `fromDependency` values are not known at admission time, their *types* are known from the publishing Definition's `output{}` / `exports{}` schema. The hub admission webhook resolves both the consuming component's `parameter{}` schema and the supplying Definition's output schema, and validates type compatibility between the two — catching broken wiring at apply time rather than at runtime.

**Example — caught at admission:**

```yaml
# aws-rds ComponentDefinition declares:
#   parameter: { port: *5432 | int }
# postgres-ha ComponentDefinition declares:
#   exports: { endpoint: string, port: int }

components:
  - name: db
    type: postgres-ha

  - name: app
    type: aws-rds
    properties:
      port:
        fromDependency:
          component: db
          path: endpoint   # ← type mismatch: endpoint is string, port expects int
                           #   caught at admission, not at runtime
```

The admission webhook rejects this with:
```
fromDependency type mismatch: db.exports.endpoint is string, aws-rds.parameter.port expects int
```

This makes the `from*` family a schema-safe wiring system — the contract between producer and consumer is enforced statically, while the values themselves remain dynamic.

### Shorthand Syntax Reference

All three directives support a shorthand string form and a map form. The shorthand is preferred for simple references; the map form is required when `default` is needed.

| Directive | Shorthand | Map form (with default) |
|---|---|---|
| `fromParameter` | `fromParameter: my-param` | `fromParameter: {name: my-param, default: "x"}` |
| `fromSource` | `fromSource: source-name.field` | `fromSource: {name: source-name, path: field, default: "x"}` |
| `fromDependency` (same topology) | `fromDependency: component.field` | `fromDependency: {component: comp, path: field, default: "x"}` |
| `fromDependency` (cross-topology) | `fromDependency: component/group.field` | `fromDependency: {component: comp, group: grp, path: field, default: "x"}` |

The parser detects form by value type — string means shorthand, map means explicit. Shorthand intentionally has no `default` support; needing a default implies the field is optional and warrants the more explicit map form.

### Summary

| Directive | Resolved from | When resolved | Schema validated at |
|---|---|---|---|
| `fromParameter` | `Application.spec.parameters` | Render time (immediate) | Admission (key + type for templated apps) |
| `fromSource` | `SourceDefinition` CueX template | Render time (lazy, cached) | Admission (path + type via `schema:` block) |
| `fromDependency` | `Component.status.exports` | After upstream healthy | Admission (path + type via `exports:` block) |

---

## Cross-KEP Requirements

### Cluster KEP — Spoke Component Admission

When spokes are modelled as a first-class `Cluster` CRD (tracked in a separate Cluster KEP), spokes should be able to declare which `ComponentDefinition` types they are willing to accept. This allows fleet operators to segment spokes by role — for example, an `infra` spoke group that only accepts cloud-provider Definitions (`aws/*`, `gcp/*`), and an `app` spoke group that only accepts workload Definitions (`webservice`, `worker`).

**Requirement for the Cluster KEP:** The `Cluster` CRD must support a component admission selector that the hub respects at dispatch time and/or the spoke enforces via admission webhook. Design details (selector mechanism, enforcement point, conflict behaviour) are deferred to that KEP.

### Definition RBAC KEP — CUE-Expressed Access Control for Definitions

Kubernetes RBAC is too coarse for controlling access to individual `ComponentDefinition`, `TraitDefinition`, `WorkflowStepDefinition`, `PolicyDefinition`, and `Dispatcher` definitions. Every time a new Definition is introduced, operators must manually update Groups, Roles, and RoleBindings — which does not scale in a platform with many teams and a growing Definition catalog.

**Requirement:** Each Definition type should support an optional accompanying access control expression, authored in CUE consistent with the Definition authoring model. The expression receives caller identity and definition context and returns a structured verdict:

```cue
// access.cue — sits alongside other Definition CUE files
access: {
    // context.user:   { name: string, groups: [...string], serviceAccount: string }
    // context.definition: { name: string, type: string, namespace: string }

    permitted: context.user.groups.exists(g, g == "platform-team") ||
               context.user.serviceAccount == "ci-deployer"
    message:   "Only platform-team members or the ci-deployer service account may use this Definition"
}
```

The access expression is evaluated by the hub admission webhook at the point a `Component`, `Application`, or `ApplicationRevision` references the Definition. Evaluation uses the same CUE runtime as Definition rendering — no separate policy engine required. The `permitted: bool` and `message: string` contract is fixed; the body is fully user-defined CUE.

This subsumes the current workaround of maintaining manual RBAC groups per definition and makes access policy a first-class, version-controlled part of the Definition itself.

**Requirement for the Definition RBAC KEP:** Design the `access.cue` evaluation lifecycle (when it runs, what context is available, caching strategy), the failure mode (fail-open vs fail-closed), and how access policies compose when a Definition is referenced transitively via Compositions or `ApplicationDefinition`.

---

## Open Questions

- [ ] Definition registry design (OCI artifact format, versioning, discovery)
- [ ] `type: component` nested instance ownership and lifecycle — does parent Component deletion cascade to child Components on the spoke?
- [ ] OAM Scope alignment — network and health scopes not addressed in this proposal
- [ ] CLI tooling for Definition authoring, linting, testing, and publishing
- [ ] Admission webhook strategy for `Component` property validation against Definition schema on the spoke
- [ ] Multi-tenancy: RBAC model for Definition consumption across namespaces
- [ ] Key storage strategy — where hub stores per-spoke signing keys (k8s Secret per spoke vs KMS)
- [ ] `run-job` output capture — convention for Jobs writing structured results back to the controller (ConfigMap written by job, read by controller after completion)
- [ ] Spoke component-controller installation and upgrade lifecycle — how is it distributed to spokes?
- [ ] Export federation for OCM — ManifestWork feedback rules for `Component.status.exports.*` fields
- [ ] `fromSource` / `fromDependency` / `fromParameter` in trait and policy properties — currently specified for component properties only; determine whether the same substitution model applies throughout
- [ ] Config distribution alignment (KEP-2.18 × KEP-2.4) — the existing Config distribution mechanism (push to target clusters) is a parallel delivery path to the Dispatcher; needs rationalisation: should platform-engineer-authored `Config` CRDs be dispatched via the same Dispatcher path as Components, or retain a separate distribution reconciler? Spoke access model (hub read via cluster-gateway vs replica on spoke), ownership when a spoke has GitOps-applied Configs of its own, and whether `SourceDefinition` cache Configs (hub-local by definition) should be excluded from distribution entirely all need design decisions before KEP-2.18 is complete.

---

## Sub-KEPs

| KEP | Title | Priority | README |
|---|---|---|---|
| KEP-2.1 | `core.oam.dev/v2alpha1` API types & CRDs | Core | [2.1-core-api/README.md](2.1-core-api/README.md) |
| KEP-2.2 | Spoke component-controller & workflow engine | Core | [2.2-spoke-controller/README.md](2.2-spoke-controller/README.md) |
| KEP-2.3 | Hub application-controller integration | Core | [2.3-hub-controller/README.md](2.3-hub-controller/README.md) |
| KEP-2.4 | Dispatcher implementations (local, cluster-gateway, OCM) | Core | [2.4-dispatchers/README.md](2.4-dispatchers/README.md) |
| KEP-2.5 | Credential model & token rotation | Core | [2.5-credential-model/README.md](2.5-credential-model/README.md) |
| KEP-2.6 | KubeVela Operator (`KubeVela` CR) | Core | [2.6-operator/README.md](2.6-operator/README.md) |
| KEP-2.7 | WorkflowRun controller bundling | Core | [2.7-workflowrun/README.md](2.7-workflowrun/README.md) |
| KEP-2.8 | KubeVela v1 → v2 migration tooling | Core | [2.8-migration/README.md](2.8-migration/README.md) |
| KEP-2.9 | `ApplicationDefinition` & `PolicyDefinition` | High | [2.9-app-policy-definitions/README.md](2.9-app-policy-definitions/README.md) |
| KEP-2.10 | Compositions | High | [2.10-compositions/README.md](2.10-compositions/README.md) |
| KEP-2.11 | Definition testing framework | High | [2.11-definition-testing/README.md](2.11-definition-testing/README.md) |
| KEP-2.12 | Definition observability integration | Enhancement | [2.12-observability/README.md](2.12-observability/README.md) |
| KEP-2.13 | Declarative Addon Lifecycle | High | [2.13-addons/README.md](2.13-addons/README.md) |
| KEP-2.14 | `TenantDefinition` & `Tenant` | High | [2.14-tenants/README.md](2.14-tenants/README.md) |
| KEP-2.15 | `OperationDefinition` & `Operation` | High | [2.15-operations/README.md](2.15-operations/README.md) |
| KEP-2.16 | `SourceDefinition` & `fromSource` | High | [2.16-source-definition/README.md](2.16-source-definition/README.md) |
| KEP-2.17 | Component Exports & same-topology `fromDependency` | High | [2.17-component-exports/README.md](2.17-component-exports/README.md) |
| KEP-2.18 | `ConfigTemplate` & `Config` CRDs | High | [2.18-config-crds/README.md](2.18-config-crds/README.md) |
| KEP-2.19 | Cross-topology `fromDependency` | High | [2.19-cross-topology-deps/README.md](2.19-cross-topology-deps/README.md) |
| KEP-2.20 | Module & API Line Versioning | High | [2.20-module-versioning/README.md](2.20-module-versioning/README.md) |
| KEP-2.21 | `from*` family resolution model | High | [2.21-from-resolution/README.md](2.21-from-resolution/README.md) |
| KEP-2.22 | Multi-Instance Addons | Medium | [2.22-multi-instance-addons/README.md](2.22-multi-instance-addons/README.md) |
