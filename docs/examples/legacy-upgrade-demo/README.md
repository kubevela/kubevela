# Legacy CUE Upgrade Demo

A self-contained example that exercises all three CUE compatibility upgrade rules
introduced in KubeVela v1.11. Each definition deliberately uses a pattern that
breaks on CUE v0.14+ and is fixed transparently at render time by the upgrade engine.

## The three issues

| File | Rule | Broken pattern | Fixed pattern |
|---|---|---|---|
| `component-db-provisioner.cue` | `list-arithmetic` | `list1 + list2`, `list * n` | `list.Concat([...])`, `list.Repeat(...)` |
| `trait-sidecar-logger.cue` | `error-field-label` | `error: "..."` (unquoted) | `"error": "..."` |
| `workflow-check-global-replica.cue` | `bool-default-negation` | `_flag: bool \| *false` + if-guard | direct boolean expression |

### Issue 1 — list arithmetic (`component-db-provisioner.cue`)

Pre-v1.11 definitions commonly use `+` to append env vars or init scripts:

```cue
allEnv: parameter.env + [{name: "MANAGED_BY", value: "kubevela"}]
expandedScripts: parameter.initScripts * parameter.scriptReplicas
```

CUE v0.14 made these a hard error. The engine rewrites them to:

```cue
allEnv: list.Concat([parameter.env, [{name: "MANAGED_BY", value: "kubevela"}]])
expandedScripts: list.Repeat(parameter.initScripts, parameter.scriptReplicas)
```

### Issue 2 — `error` field label (`trait-sidecar-logger.cue`)

CUE v0.14 introduced `error` as a built-in function, breaking any field named `error`:

```cue
// parse error in CUE v0.14+
error: "unsupported log format; expected json or logfmt"
```

The engine quotes the field label:

```cue
"error": "unsupported log format; expected json or logfmt"
```

### Issue 3 — bool default negation (`workflow-check-global-replica.cue`)

CUE v0.14 reads the default value of a `bool | *false` field before unification,
so `if !_isSecondary` fires even when `_isSecondary` was set to `true` by a
conditional block. This causes secondary clusters to incorrectly fail the
`engineVersion` requirement:

```cue
// Broken: default evaluated before unification in CUE v0.14+
_isSecondary: bool | *false
if parameter.globalCluster != _|_ {
    if parameter.globalCluster.mode == "secondary" {
        _isSecondary: true
    }
}
if !_isSecondary {
    if parameter.engineVersion == _|_ {
        _engineVersionRequired: 0 & "engineVersion is required for primary clusters"
    }
}
```

The engine inlines the conditions directly into the flag value:

```cue
_isSecondary: (parameter.globalCluster != _|_ && parameter.globalCluster.mode == "secondary")
if !_isSecondary {
    if parameter.engineVersion == _|_ {
        _engineVersionRequired: 0 & "engineVersion is required for primary clusters"
    }
}
```

## The Application

`application.yaml` deploys a secondary database replica that exercises all three
definitions simultaneously:

- **Component** `db-provisioner` — Deployment with env var concatenation and init script repetition
- **Trait** `sidecar-logger` — injects a Fluent Bit sidecar and emits a status ConfigMap with an `error` field
- **Workflow step** `check-global-replica` — validates replica role; `engineVersion` is intentionally
  omitted to prove the bool-default-negation fix works (a secondary must not require it)

## Usage

```bash
# 1. Apply the three definitions
vela def apply examples/legacy-upgrade-demo/component-db-provisioner.cue
vela def apply examples/legacy-upgrade-demo/trait-sidecar-logger.cue
vela def apply examples/legacy-upgrade-demo/workflow-check-global-replica.cue

# 2. Deploy the application
kubectl apply -f examples/legacy-upgrade-demo/application.yaml

# 3. Watch it reconcile — all three definitions are upgraded transparently
kubectl get application db-global-secondary -w

# 4. Confirm the workflow step result ConfigMap was created
kubectl get configmap replica-check-result -o yaml
# Expected: data.isSecondary=true, no engineVersion key

# 5. To see the raw failures, restart the controller with the flag disabled:
#      --enable-cue-version-compatibility=false
#    All three definitions will produce CUE compilation errors.
```

## Upgrading definitions permanently

To fix the definitions at source rather than relying on the runtime compatibility layer:

```bash
vela def upgrade examples/legacy-upgrade-demo/component-db-provisioner.cue
vela def upgrade examples/legacy-upgrade-demo/trait-sidecar-logger.cue
vela def upgrade examples/legacy-upgrade-demo/workflow-check-global-replica.cue
```
