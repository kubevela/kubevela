# KEP-2.18: ConfigTemplate & Config as First-Class CRDs

**Status:** Drafting (Not ready for consumption)
**Parent:** [vNext Roadmap](../README.md)

`ConfigTemplate` and `Config` are currently implemented as formatted `ConfigMap` objects identified by a well-known label convention. The CUE authoring model is good — a `metadata` block and a `template` block containing a `parameter` schema and an output shape — but the storage is not. Schema validation only occurs in the CLI (`vela config`); the API server accepts any ConfigMap regardless of content. GitOps adoption is poor — authors must hand-craft ConfigMaps with CUE embedded as string values in `data` keys, and `kubectl apply` provides no feedback on malformed entries.

This KEP promotes both to proper CRDs while preserving the existing CUE authoring model exactly.

## Problem

- **No admission-time validation** — malformed ConfigMap-format entries are accepted by the API server and only caught at runtime
- **Poor GitOps ergonomics** — the CUE template is stored as a raw string inside a ConfigMap `data` key; there is no native YAML structure and no API server awareness of the content
- **Weak RBAC** — access control relies on label selectors against the ConfigMap resource type rather than a dedicated API group and resource name
- **No status model** — ConfigMaps have no `status` subresource; controllers cannot surface validation errors or last-updated timestamps
- **No GC integration** — `ownerReferences` on ConfigMaps work but are unconventional; CRD-backed resources integrate naturally with Kubernetes garbage collection

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

**Config CRD** — namespace-scoped. Holds the user-supplied `properties` map validated against the referenced `ConfigTemplate`'s `parameter` schema at admission.

```yaml
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
    content:     { key: value }
    contentType: json
status:
  phase:      Valid        # Valid | Invalid | Pending
  message:    ""
  lastSyncAt: "2026-03-29T10:00:00Z"
```

## Migration Path

The v1 `vela config` CLI and application-controller continue to support ConfigMap-format entries during the transition period. A `vela config migrate` command converts existing ConfigMap-based entries to `ConfigTemplate` + `Config` CRDs in-place. The migration is non-destructive — original ConfigMaps are retained with a `config.oam.dev/migrated: "true"` annotation until the operator explicitly removes them.

The controller reads CRD-format entries preferentially; on miss it falls back to ConfigMap-format. The fallback is removed at vNext GA.

## Relationship to SourceDefinition Caching (KEP-2.16)

The versioned `ConfigTemplate` CRDs created by the application-controller for `SourceDefinition` cache entries (`<definition-name>-v<schema-hash>`) are controller-managed. The application-controller has permission to create and update them; application authors do not. This enforces the read-only cache semantics described in KEP-2.16 — resolved source values are observable via `kubectl get config` but not writable by users without elevated RBAC.

## Key Design Decisions

- **`config.oam.dev` API group** — separate from `core.oam.dev` to allow the Config subsystem to version independently
- **Cluster-scoped ConfigTemplate, namespace-scoped Config** — platform engineers own the schema; tenants own their data instances
- **CUE authoring model unchanged** — `metadata` + `template { parameter{} ... }` format is identical to today; only the storage backing changes
- **Schema hash in controller-managed ConfigTemplate names** — prevents silent type mismatches when a `SourceDefinition` output schema changes (see KEP-2.16)

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

The existing Config system has its own distribution mechanism for pushing Config entries to target clusters. In the v2 model, `Config` objects that are referenced by topology groups are hub-side objects — the hub reads them during dispatch orchestration and they do not need to be distributed to spokes. `Config` objects that represent runtime platform data (e.g., nacos config, addon configuration) may still need distribution; this aligns with the Dispatcher model in KEP-2.4 where the active Dispatcher is responsible for delivering Config-derived data alongside `Component` CRs. Full rationalisation of the Config distribution path is tracked as a follow-on to KEP-2.4.
