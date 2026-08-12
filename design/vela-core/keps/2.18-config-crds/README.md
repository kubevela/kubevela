> ⚠️ **Early concept draft.** This KEP is an early-stage exploration. It is **incomplete and may be inaccurate**, its direction is unsettled, and it should not be relied upon for implementation or as a description of committed behaviour. Expect substantial change.

# KEP-2.18: ConfigTemplate & Config as First-Class CRDs

**Status:** Drafting (Not ready for consumption)
**Parent:** [vNext Roadmap](../README.md)

`ConfigTemplate` and `Config` are currently implemented as formatted `ConfigMap` objects identified by a well-known label convention. The CUE authoring model is good — a `metadata` block and a `template` block containing a `parameter` schema and an output shape — but the storage is not. Schema validation only occurs in the CLI (`vela config`); the API server accepts any ConfigMap regardless of content. GitOps adoption is poor — authors must hand-craft ConfigMaps with CUE embedded as string values in `data` keys, and `kubectl apply` provides no feedback on malformed entries.

This KEP promotes both to proper CRDs while preserving the existing CUE authoring model exactly.

## Problem

- **No admission-time validation** — malformed ConfigMap-format entries are accepted by the API server and only caught at runtime
- **Poor GitOps ergonomics** — the CUE template is stored as a raw string inside a ConfigMap `data` key; Config properties must be base64-encoded JSON in a Secret `data` key; neither has native YAML structure or API server awareness
- **Weak RBAC** — access control relies on label selectors against the ConfigMap resource type rather than a dedicated API group and resource name
- **No status model** — ConfigMaps have no `status` subresource; controllers cannot surface validation errors or last-updated timestamps
- **No GC integration** — `ownerReferences` on ConfigMaps work but are unconventional; CRD-backed resources integrate naturally with Kubernetes garbage collection
- **No drift correction** — resources created by a Config's `output:` block are applied once and never reconciled; mutations or deletions are not detected or corrected
- **No reactive updates** — credential rotation or Secret changes require manual re-triggering via `vela config create`; there is no watch-based propagation

## API Design

The CUE authoring format is unchanged — `metadata` block and `template` block with `parameter` schema and output shape. The CRD simply becomes the native storage for what today gets embedded into a ConfigMap.

**ConfigTemplate CRD** — cluster-scoped. Static metadata (`alias`, `description`, `sensitive`, `scope`) are first-class `spec` fields. The CUE template in `spec.template` contains only the executable logic — the `parameter` schema and output shape. The controller extracts the `parameter` schema and validates `Config` instances against it at admission.

```yaml
apiVersion: config.oam.dev/v1beta1
kind: ConfigTemplate
metadata:
  name: nacos-config
spec:
  alias:       "Nacos Configuration"
  description: "Write the configuration to the nacos"
  sensitive:   false
  scope:       system
  template: |
    nacos: {
      endpoint: { name: "nacos" }
      format: parameter.contentType
      metadata: {
        dataId: parameter.dataId
        group:  parameter.group
      }
      content: parameter.content
    }
    parameter: {
      dataId:      string
      group:       *"DEFAULT_GROUP" | string
      content:     { ... }
      contentType: *"json" | "yaml" | "properties" | "toml"
      appName?:    string
    }
```

**Config CRD** — namespace-scoped. Holds user-supplied parameter values validated against the referenced `ConfigTemplate`'s `parameter` schema. Non-sensitive values are supplied inline in `spec.properties`; sensitive values are kept in a separately managed Secret and referenced via `spec.fromSecrets`.

```yaml
# Non-sensitive config — all values inline
apiVersion: config.oam.dev/v1beta1
kind: Config
metadata:
  name: helm-repo
  namespace: my-app-ns
spec:
  template: helm-repository
  properties:
    url:  https://charts.example.com
    name: example
status:
  phase:      Valid        # Valid | Invalid | Pending
  message:    ""
  lastSyncAt: "2026-03-29T10:00:00Z"
```

```yaml
# Mixed: non-sensitive inline, sensitive/external from Secrets and ConfigMaps
apiVersion: config.oam.dev/v1beta1
kind: Config
metadata:
  name: my-nacos-config
  namespace: my-app-ns
spec:
  template: nacos-config
  properties:
    dataId:      app-config
    group:       DEFAULT_GROUP
    contentType: json
  fromSecrets:
    - name: nacos-credentials
      keys:
        - key: password
          property: serverPassword
        - key: username
          property: serverUsername
  fromConfigMaps:
    - name: nacos-endpoint
      keys:
        - key: host
          property: serverHost
        - key: port
          property: serverPort
status:
  phase:      Valid
  message:    ""
  lastSyncAt: "2026-03-29T10:00:00Z"
```

`fromSecrets` and `fromConfigMaps` are lists of references resolved by the controller and merged with `spec.properties` before CUE template rendering; `spec.properties` takes precedence on key collision. All references are restricted to the same namespace as the Config (except for Configs in `vela-system`, which may reference any namespace). This keeps the Config compiler sandboxed — templates do not need `http` or `kube` providers to consume external data managed by other controllers or Helm charts.

If a referenced Secret or ConfigMap does not exist at reconcile time, the Config transitions to `status.phase: Pending` until it becomes available — allowing manifests to land in different GitOps sync waves.

### `// +sensitive` marker

ConfigTemplate authors mark sensitive parameters with a `// +sensitive` comment in the `parameter` block, using the same annotation convention as `// +immutable`:

```cue
parameter: {
  host:     string
  username: string
  // +sensitive
  password: string
}
```

`vela config create` reads these markers and automatically splits the supplied values:
- Non-sensitive values are written inline to `spec.properties`
- Sensitive values are written to a generated Secret (`<config-name>-sensitive`) and the Config CR is pre-populated with the corresponding `fromSecrets` entries

This means authors using the CLI never need to think about the split — they supply all values as flags and the CLI handles separation. For GitOps workflows, authors create the Config CR directly with their own Secret (Sealed Secret, ESO etc.) referenced in `fromSecrets`; the `// +sensitive` marker serves as documentation for which fields are expected there.

`vela config-template show` surfaces the marker so developers know which fields to expect in Secrets vs inline properties.

## Config Controller

A Config controller is introduced by this KEP (no equivalent exists today — the current system is factory-based with synchronous CRUD calls and no reconciliation loop). The controller:

- Watches `Config` CRDs
- Watches Secrets referenced by `spec.fromSecrets` via an index — a Secret update immediately enqueues any Config that references it
- Watches output objects produced by `output:` / `outputs:` blocks via owner references — deletion or mutation of an output object immediately enqueues the owning Config for re-reconciliation
- On each reconcile: reads `spec.properties`, resolves all `spec.fromSecrets` references, merges the results, and validates the merged property set against the `ConfigTemplate` parameter schema
- Re-renders the CUE template and re-applies `output:` / `outputs:` objects on every reconcile loop (drift correction — same model as the application controller for components)
- Re-triggers ExpandedWriter destinations (Nacos etc.) when the rendered output changes
- Sets `status.phase`: `Pending` (a referenced Secret is missing), `Invalid` (schema validation failed with detail in `status.message`), or `Valid`

This replaces the current `ParseConfig` + `CreateOrUpdateConfig` + `writer.Write` synchronous path with a continuously-reconciling model. Credential rotation propagates automatically: when a referenced Secret is updated, the controller picks up the change, re-renders, and re-applies without any manual intervention.

The existing `Factory` interface methods are retained for use by workflow CUE providers and the CLI, but their write path delegates to the controller's reconciliation state rather than performing independent writes.

## Migration Path

The v1 `vela config` CLI and application-controller continue to support ConfigMap-format entries during the transition period. A `vela config migrate` command converts existing entries to `ConfigTemplate` + `Config` CRDs in-place.

Migration is purely additive — no data is moved or re-encoded:

- For each legacy Config Secret, a `Config` CRD is created with `spec.fromSecrets` referencing the existing Secret, mapping all keys. The original Secret is annotated with `config.oam.dev/migrated: "true"` and `config.oam.dev/config-ref: <namespace>/<name>` pointing at the newly created Config CR, making the relationship navigable in both directions.
- Since the original Secret's sensitivity is unknown, all migrations treat it as sensitive and use `fromSecrets`. Operators can later move non-sensitive values inline to `spec.properties` at their discretion.
- ConfigTemplate ConfigMaps are converted to `ConfigTemplate` CRDs with `spec.alias`, `spec.description`, `spec.sensitive`, and `spec.scope` extracted from the CUE `metadata` block into first-class fields.
- `secret.Data["objects-reference"]` entries migrate to controller-managed owned resources tracked via the Config CRD's status.

The controller reads CRD-format entries preferentially; on miss it falls back to ConfigMap/Secret-format. The fallback is removed at vNext GA.

## Relationship to SourceDefinition Caching (KEP-2.16)

The versioned `ConfigTemplate` CRDs created by the application-controller for `SourceDefinition` cache entries (`<definition-name>-v<schema-hash>`) are controller-managed. The application-controller has permission to create and update them; application authors do not. This enforces the read-only cache semantics described in KEP-2.16 — resolved source values are observable via `kubectl get config` but not writable by users without elevated RBAC.

## Key Design Decisions

- **`config.oam.dev` API group** — separate from `core.oam.dev` to allow the Config subsystem to version independently
- **Cluster-scoped ConfigTemplate, namespace-scoped Config** — platform engineers own the schema; tenants own their data instances
- **CUE authoring model unchanged** — `metadata` + `template { parameter{} ... }` format is identical to today; only the storage backing changes
- **Schema hash in controller-managed ConfigTemplate names** — prevents silent type mismatches when a `SourceDefinition` output schema changes (see KEP-2.16)
- **`spec.properties` + `spec.fromSecrets` + `spec.fromConfigMaps`** — sensitive values stay in Secrets (preserving Secret RBAC and etcd at-rest encryption); non-sensitive values are inline for readable GitOps diffs. The split is template-driven via `// +sensitive` markers on `parameter` fields. `vela config create` automates the split; GitOps authors reference their own Secrets directly. Mirrors the Pod `env`/`envFrom` pattern already familiar to Kubernetes authors.
- **Config controller replaces the factory's synchronous write path** — the existing factory pattern is push-based with no drift correction and no watch-based propagation. A reconciler provides both naturally. The factory interface is preserved for workflow providers and the CLI but its write path routes through the controller's desired-state model.
- **Scope derived from namespace** — `vela-system` Configs are system-scoped and may reference Secrets/ConfigMaps in any namespace; Configs in other namespaces are project-scoped and restricted to same-namespace references. The `scope` field on ConfigTemplate is retained for backwards-compatible listing but is not author-settable on Config instances — it is derived and set by the controller. This enforces the system/project boundary that was previously advisory-only.

## Alternatives Considered

### Aggregated API server over existing ConfigMap/Secret storage

An aggregated API (AA) server could present `ConfigTemplate` and `Config` as typed Kubernetes resources while keeping ConfigMaps and Secrets as the backing store, avoiding any data migration. This was considered and rejected for two reasons.

First, Config and ConfigTemplate are not virtual or synthesized resources — they are structured data with no reason to avoid etcd. The precedent in this codebase for AA is `VirtualCluster` (cluster-gateway), which justifies its AA complexity because it must proxy arbitrary API calls to remote clusters and aggregates data from Secrets and OCM ManagedClusters at query time. That use case cannot be served by a CRD. Config/ConfigTemplate has no equivalent requirement.

Second, an AA server carries permanent operational overhead: a separate process, TLS bootstrapping, an `APIService` registration, and an availability dependency separate from kube-apiserver. The migration cost for the CRD approach is one-time and non-destructive (original ConfigMaps are retained with an annotation until explicitly removed). Trading a one-time migration for permanent operational complexity is the wrong trade.

### Validating webhook over existing ConfigMap/Secret storage

A validating admission webhook on labeled Secrets and ConfigMaps would move schema validation from the CLI to the API server without changing storage or introducing new resource types. This was considered and rejected because it does not address the primary GitOps authoring pain point: authors would still hand-craft Secrets with base64-encoded `input-properties` and ConfigMaps with CUE embedded as raw strings. Validation errors surface earlier, but the authoring format remains opaque in PR diffs and error-prone to write without tooling. The CRD approach fixes both validation and authoring ergonomics simultaneously.

## Non-Goals

- Replacing Kubernetes `Secret` for credential storage — `sensitive: true` entries are for non-credential platform metadata; credentials belong in Secrets or an external secrets manager
- A general-purpose key-value store — Config is scoped to KubeVela platform data (source cache, addon config, platform metadata)

## Distribution (Draft Thoughts)

The current distribution mechanism (`vela config distribute`) creates a proxy `Application` using `ref-objects` to ship the raw Config Secret to target clusters via topology policies. This has two problems: it clutters the Application list with infrastructure noise, and spoke clusters receive a raw Secret they cannot independently process (they would need the ConfigTemplate and a local controller to render it).

A better model: `Config` specifies its own `spec.topology` and the Config controller dispatches the **rendered output objects** directly to target clusters — not the Config Secret itself. Spokes receive ready-to-use resources with no local controller dependency. Distribution status folds into the Config's own `status`.

Whether the Config controller dispatches directly or delegates to the Dispatcher (KEP-2.4) is TBD — KEP-2.4 is the natural owner of multi-cluster delivery semantics so delegation is likely the right split.

The existing `ExpandedWriter` mechanism (currently hardcoded to Nacos) is also a form of distribution — pushing rendered config to an external system rather than a spoke cluster. This should eventually be made extensible (writer plugins rather than hardcoded keys) but that is out of scope for this KEP.
