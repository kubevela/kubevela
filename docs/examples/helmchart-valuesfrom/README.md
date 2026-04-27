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
| `namespace`  | Chart release namespace (which itself defaults to the Application's own namespace when `release.namespace` is unset) | Cross-namespace references are **rejected** by design — the explicit value must equal either the release namespace or the Application's own namespace. |
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

### External CM/Secret edits propagate on the next reconcile

An edit to a referenced `ConfigMap` or `Secret` does not directly trigger a
reconcile, but on the next reconcile (periodic resync, default ~5 minutes) the
controller computes a content fingerprint of every referenced source and folds
it into `desiredRev` as a `-vf-…` suffix on `status.workflow.appRevision`. When
the content moves, the suffix moves, the workflow gate fails, the workflow
restarts, and the chart re-renders with the new values.

To roll out immediately rather than waiting for the next resync, touch the
`app.oam.dev/requestreconcile` annotation on the Application:

```bash
kubectl -n myapp annotate app foo app.oam.dev/requestreconcile=$(date +%s) --overwrite
```

Caveats:

- **Cosmetic-only edits do not roll out.** YAML→JSON canonicalisation (matching
  Helm's own value-comparison contract) drops whitespace, comments, and key
  order, so an edit that adds a comment or reformats produces the same digest.
- **A rollback to identical earlier content does not roll out**, for the same
  reason.
- **`publishVersion` annotation pin**: when `app.oam.dev/publishVersion` is set,
  the suffix is **not** appended. The pin is hard — CM/Secret edits are
  deferred until the user bumps the pin.
- **Multi-cluster**: `valuesFrom` sources are always read from the
  control-plane cluster, regardless of where the chart is deployed (e.g. via a
  topology policy). A CM/Secret living only on a target member cluster will not
  be found.
- **Optional missing sources** contribute a stable `<missing>` sentinel to the
  fingerprint, so they do not cause spurious upgrades while absent and
  deterministically move the digest the moment they appear.

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
