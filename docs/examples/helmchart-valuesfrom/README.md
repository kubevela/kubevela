# helmchart valuesFrom examples

The native `helmchart` component can merge values from `ConfigMap` and `Secret`
resources before rendering the chart. Useful for pulling per-environment config,
database DSNs, or any sensitive value out of the Application spec itself.

## Quick reference

```yaml
components:
  - type: helmchart
    properties:
      chart:
        source: podinfo
        repoURL: https://stefanprodan.github.io/podinfo
        version: "6.11.1"
      release:
        name: podinfo
        namespace: myapp
      values:
        replicaCount: 3              # inline — wins over anything below
      valuesFrom:
        - kind: ConfigMap
          name: podinfo-base         # read first
        - kind: Secret
          name: podinfo-overlay      # read second, overrides earlier on conflict
          # key: values.yaml         # optional, defaults to "values.yaml"
          # optional: true           # skip silently on not-found
```

## Merge order

Highest priority wins:

```
inline `values` > valuesFrom[N] > valuesFrom[N-1] > ... > valuesFrom[0] > chart defaults
```

- **Deep-merge** for map keys (e.g. `resources.limits.memory`)
- **Replace** for arrays (e.g. `extraArgs: [...]` from a later source replaces earlier)
- `null` values are preserved (not treated as a delete marker)

## Semantics you should know

| Field        | Default                        | Notes                                                      |
|--------------|--------------------------------|------------------------------------------------------------|
| `kind`       | —                              | Only `Secret` and `ConfigMap` are supported.               |
| `name`       | —                              | Required.                                                  |
| `namespace`  | Application's own namespace    | Cross-namespace references are **rejected** by design.     |
| `key`        | `values.yaml`                  | Which key inside `.data` holds the YAML blob.              |
| `optional`   | `false`                        | When `true`, missing resource/key is skipped silently. Parse errors and permission errors still fail. |

### Cross-namespace is disallowed

Because the KubeVela controller has cluster-wide read on ConfigMaps and Secrets,
allowing a user-supplied `namespace` would let any tenant read arbitrary
Secrets. If the namespace you specify differs from the Application's own
namespace, the reconcile fails with:

```
cross-namespace valuesFrom sources are not permitted
```

If you need shared config across namespaces, copy the ConfigMap into each
Application namespace (e.g. via a replicator) rather than referencing across.

### External CM/Secret edits do not auto-reconcile

Editing a referenced ConfigMap/Secret does **not** trigger a reconcile on the
Application. Either:

1. Touch the Application spec (any annotation/field under `.spec` works) to
   force a new revision, or
2. Wait for the next periodic resync.

This mirrors the KubeVela model where the Application spec is the source of
truth. Tools that want live updates should rebuild a sibling `ConfigMap` with a
content-hash name and point the Application at it.

## Files in this folder

- [`configmap-backed.yaml`](./configmap-backed.yaml) — a minimal example with
  one ConfigMap backing `replicaCount`.
- [`secret-and-inline.yaml`](./secret-and-inline.yaml) — three-layer merge:
  chart defaults → ConfigMap → Secret → inline.

Each file is self-contained (the ConfigMap/Secret is bundled alongside the
Application). Apply with:

```bash
kubectl apply -f configmap-backed.yaml
# or
kubectl apply -f secret-and-inline.yaml
```
