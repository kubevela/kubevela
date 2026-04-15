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

**ConfigTemplate CRD** — cluster-scoped. Stores the CUE template directly in `spec.template`. The controller extracts the `parameter` schema and validates `Config` instances against it at admission.

```yaml
apiVersion: config.oam.dev/v1beta1
kind: ConfigTemplate
metadata:
  name: nacos-config
spec:
  template: |
    metadata: {
      name:        "nacos-config"
      alias:       "Nacos Configuration"
      description: "Write the configuration to the nacos"
      sensitive:   false
      scope:       "system"
    }

    template: {
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
# Mixed: non-sensitive inline, sensitive from a Secret
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
    - name: nacos-credentials   # author-managed: Sealed Secret, ESO, etc.
status:
  phase:      Valid
  message:    ""
  lastSyncAt: "2026-03-29T10:00:00Z"
```

`fromSecrets` is a list of Secret references. The controller reads each referenced Secret and merges its keys with `spec.properties` before CUE template rendering; `spec.properties` takes precedence on key collision. This mirrors the Pod `env`/`envFrom` pattern and keeps sensitive values readable in GitOps diffs (non-sensitive) while preserving Secret RBAC and etcd encryption for credentials.

If a referenced Secret does not exist at reconcile time, the Config transitions to `status.phase: Pending` until the Secret becomes available — allowing Config and Secret manifests to land in different GitOps sync waves.

## Config Controller

A Config controller is introduced by this KEP (no equivalent exists today — the current system is factory-based with synchronous CRUD calls and no reconciliation loop). The controller:

- Watches `Config` CRDs
- Watches Secrets referenced by `spec.fromSecrets` via an index — a Secret update immediately enqueues any Config that references it
- On each reconcile: reads `spec.properties`, resolves all `spec.fromSecrets` references, merges the results, and validates the merged property set against the `ConfigTemplate` parameter schema
- Re-renders the CUE template and re-applies `output:` / `outputs:` objects on every reconcile loop (drift correction — same model as the application controller for components)
- Re-triggers ExpandedWriter destinations (Nacos etc.) when the rendered output changes
- Sets `status.phase`: `Pending` (a referenced Secret is missing), `Invalid` (schema validation failed with detail in `status.message`), or `Valid`

This replaces the current `ParseConfig` + `CreateOrUpdateConfig` + `writer.Write` synchronous path with a continuously-reconciling model. Credential rotation propagates automatically: when a referenced Secret is updated, the controller picks up the change, re-renders, and re-applies without any manual intervention.

The existing `Factory` interface methods are retained for use by workflow CUE providers and the CLI, but their write path delegates to the controller's reconciliation state rather than performing independent writes.

## Migration Path

The v1 `vela config` CLI and application-controller continue to support ConfigMap-format entries during the transition period. A `vela config migrate` command converts existing entries to `ConfigTemplate` + `Config` CRDs in-place. The migration is non-destructive — original ConfigMaps and Secrets are retained with a `config.oam.dev/migrated: "true"` annotation until the operator explicitly removes them.

Migration handles both cases:
- **Non-sensitive Secrets** — `vela config migrate` reads `secret.Data["input-properties"]`, creates a `Config` CRD with inline `spec.properties`, and annotates the original Secret
- **Sensitive Secrets** — `vela config migrate` creates a `Config` CRD with `spec.fromSecrets` referencing the existing Secret; the Secret is retained as the authoritative property store, not merely as a migration artifact
- **OutputObject references** — `secret.Data["objects-reference"]` entries migrate to controller-managed owned resources tracked via the Config CRD's status

The controller reads CRD-format entries preferentially; on miss it falls back to ConfigMap/Secret-format. The fallback is removed at vNext GA.

## Relationship to SourceDefinition Caching (KEP-2.16)

The versioned `ConfigTemplate` CRDs created by the application-controller for `SourceDefinition` cache entries (`<definition-name>-v<schema-hash>`) are controller-managed. The application-controller has permission to create and update them; application authors do not. This enforces the read-only cache semantics described in KEP-2.16 — resolved source values are observable via `kubectl get config` but not writable by users without elevated RBAC.

## Key Design Decisions

- **`config.oam.dev` API group** — separate from `core.oam.dev` to allow the Config subsystem to version independently
- **Cluster-scoped ConfigTemplate, namespace-scoped Config** — platform engineers own the schema; tenants own their data instances
- **CUE authoring model unchanged** — `metadata` + `template { parameter{} ... }` format is identical to today; only the storage backing changes
- **Schema hash in controller-managed ConfigTemplate names** — prevents silent type mismatches when a `SourceDefinition` output schema changes (see KEP-2.16)
- **`spec.properties` + `spec.fromSecrets`** — sensitive values stay in Secrets (preserving Secret RBAC and etcd at-rest encryption); non-sensitive values are inline for readable GitOps diffs. The split is author-driven per-field, not enforced by the ConfigTemplate's `sensitive` flag. Mirrors the Pod `env`/`envFrom` pattern already familiar to Kubernetes authors.
- **Config controller replaces the factory's synchronous write path** — the existing factory pattern is push-based with no drift correction and no watch-based propagation. A reconciler provides both naturally. The factory interface is preserved for workflow providers and the CLI but its write path routes through the controller's desired-state model.

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

## Dispatcher-Owned ConfigTemplate Registration

A special class of `ConfigTemplate` is owned by Dispatcher implementations rather than platform engineers. Each Dispatcher ships a `topology.cue` schema that is registered as a `ConfigTemplate` automatically when the Dispatcher is installed via the `KubeVela` operator.

These controller-managed ConfigTemplates are distinguished by a label:

```yaml
metadata:
  labels:
    config.oam.dev/managed-by: dispatcher
    config.oam.dev/dispatcher: cluster-gateway
```

Platform engineers create `Config` instances against these templates to define named topology groups (see KEP-2.4 and KEP-2.17). The operator ensures the template exists before any Application is processed — a missing dispatcher topology ConfigTemplate surfaces as a degraded condition on the `KubeVela` CR, not a silent runtime error.

Dispatcher topology ConfigTemplates are cluster-scoped and treated as immutable by platform engineers — updates to the schema come only via Dispatcher upgrades, not manual edits.

## Distribution Alignment with KEP-2.4

The existing Config system has its own distribution mechanism for pushing Config entries to target clusters. In the vNext model, `Config` objects that are referenced by topology groups are hub-side objects — the hub reads them during dispatch orchestration and they do not need to be distributed to spokes. `Config` objects that represent runtime platform data (e.g., nacos config, addon configuration) may still need distribution; this aligns with the Dispatcher model in KEP-2.4 where the active Dispatcher is responsible for delivering Config-derived data alongside `Component` CRs. Full rationalisation of the Config distribution path is tracked as a follow-on to KEP-2.4.
