# KEP-2.21: The `from*` Family — Render-Time Resolution & Schema Validation

**Status:** Drafting (Not ready for consumption)
**Parent:** [vNext Roadmap](../README.md)
**Depends on:** [KEP-2.1](../2.1-core-api/README.md), [KEP-2.3](../2.3-hub-controller/README.md)
**Consumed by:** [KEP-2.16](../2.16-source-definition/README.md) (`fromSource`), [KEP-2.17](../2.17-component-exports/README.md) (`fromDependency`), [KEP-2.19](../2.19-cross-topology-deps/README.md) (cross-topology `fromDependency`)

`fromParameter`, `fromSource`, and `fromDependency` are a family of inline value substitution directives. This KEP specifies the resolution model, admission-time schema validation, and shorthand syntax that apply to all three. The specific semantics of each directive — what it resolves from and its authoring model — are covered in the individual KEPs; this KEP covers the cross-cutting mechanics they share.

## Summary

| Directive | Resolved from | When resolved | Schema validated at |
|---|---|---|---|
| `fromParameter` | `Application.spec.parameters` | Render time (immediate) | Admission (key + type for templated apps) |
| `fromSource` | `SourceDefinition` CueX template | Render time (lazy, cached) | Admission (path + type via `schema:` block) |
| `fromDependency` | `Component.status.exports` | After upstream healthy | Admission (path + type via `exports:` block) |

## Resolution Model

Resolution happens in two phases, both before component rendering:

**Phase 1 — Source resolution (sequential):** `spec.sources` entries are resolved in declaration order. For each source, any `fromSource` references in its `properties` are resolved against already-resolved earlier sources. The resolved source output is cached and made available for subsequent sources and for component properties. This enables source chaining — see KEP-2.16 for full details.

**Phase 2 — Component property resolution (single recursive pass):** The Application controller performs a single recursive pass over the merged `properties` map for each component immediately before handing the properties to the Definition renderer. The traversal:

1. Walks every node in the properties tree (objects, arrays, scalar leaves)
2. When it encounters a node whose **sole key** is `fromParameter`, `fromSource`, or `fromDependency`, it replaces the entire node with the resolved value
3. Passes the fully-resolved properties to the CUE Definition renderer

`fromDependency` nodes are only resolvable once the referenced component is healthy and has written its `exports` to `Component.status.exports`. Until then, the consuming component is not rendered — the controller defers it and re-queues when the upstream export becomes available. `fromSource` nodes are resolved using the already-cached source outputs from Phase 1. `fromParameter` nodes are resolved immediately from `spec.parameters`.

The Phase 2 recursive pass is deterministic and single-pass for `fromParameter` and `fromSource`. It is topologically ordered for `fromDependency`.

## Detection

`from*` directives are detected **structurally** — not by string matching. A node is detected as a substitution directive if and only if it is a map containing exactly one key and that key is `fromParameter`, `fromSource`, or `fromDependency`. Any map with additional keys alongside one of these is treated as a plain map, not a substitution directive.

```yaml
# Detected as a fromParameter directive
region:
  fromParameter: region

# NOT detected — has a second key alongside fromParameter
region:
  fromParameter: region
  default: us-east-1    # incorrect — use map form instead
```

`from*` directives are valid at any depth within `properties`, including nested objects and array entries. They are not valid as map keys, only as values.

## Schema Contract Validation at Admission Time

The `from*` directives are deliberately declarative. This means the *schema* of every substitution reference can be validated at `kubectl apply` time — before reconciliation, before rendering, and before any runtime values exist.

| Directive | Schema source | Validated at admission |
|---|---|---|
| `fromParameter` | `spec.parameters` (free-form) or `ApplicationDefinition.parameter{}` schema | Key existence (free-form) or key type match (templated) |
| `fromSource` | `SourceDefinition.schema:` block | Source name exists in `spec.sources`; `path` refers to a declared output field; output field type is compatible with the consuming component's `parameter{}` schema at the given property path |
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

## Shorthand Syntax

All three directives support a shorthand string form and a map form. The shorthand is preferred for simple references; the map form is required when `default` is needed.

| Directive | Shorthand | Map form (with default) |
|---|---|---|
| `fromParameter` | `fromParameter: my-param` | `fromParameter: {name: my-param, default: "x"}` |
| `fromSource` | `fromSource: source-name.field` | `fromSource: {name: source-name, path: field, default: "x"}` |
| `fromDependency` (same topology) | `fromDependency: component.field` | `fromDependency: {component: comp, path: field, default: "x"}` |
| `fromDependency` (cross-topology) | `fromDependency: component/group.field` | `fromDependency: {component: comp, group: grp, path: field, default: "x"}` |

The parser detects form by value type — string means shorthand, map means explicit. Shorthand intentionally has no `default` support; needing a default implies the field is optional and warrants the more explicit map form.

## fromParameter

`fromParameter` resolves from `Application.spec.parameters` — a free-form set of values representing application-level constants shared across components, traits, policies, and operations.

```yaml
spec:
  parameters:
    region: us-west-2
    tier: production

  components:
    - name: my-db
      type: aws-rds
      properties:
        region:
          fromParameter: region
        tier:
          fromParameter: tier
```

`fromParameter` is valid within `properties` at any depth. It is not valid in `OperationDefinition` templates — Operations use `context.appParams` directly.

For templated Applications (those referencing an `ApplicationDefinition`), `fromParameter` references are validated against the Definition's `parameter{}` schema at admission. For non-templated Applications, key existence is checked.

## fromSource

`fromSource` resolves from a `SourceDefinition` instance declared in `Application.spec.sources`. It supports lazy, cached resolution via the `Config` CRD. Full authoring model in [KEP-2.16](../2.16-source-definition/README.md).

```yaml
spec:
  sources:
    - name: cluster-info
      definition: cluster-config-reader
      properties:
        cacheDuration: "1h"

  components:
    - name: api
      type: webservice
      properties:
        region:
          fromSource: cluster-info.region
```

## fromDependency

`fromDependency` resolves from a sibling component's `Component.status.exports`. Creates an implicit ordering dependency — the consuming component is not rendered until the producing component is healthy. Full authoring model in [KEP-2.17](../2.17-component-exports/README.md). Cross-topology resolution in [KEP-2.19](../2.19-cross-topology-deps/README.md).

```yaml
components:
  - name: database
    type: postgres

  - name: api
    type: webservice
    properties:
      dbHost:
        fromDependency: database.endpoint
```

## Non-Goals

- Arbitrary runtime property mutation (these are render-time, not reconcile-time)
- Cross-application substitution — `fromDependency` is intra-Application only
- `from*` as map keys — values only

## Cross-KEP References

- **KEP-2.3** — Hub reconcile pipeline; the resolution pass runs in the hub application-controller
- **KEP-2.9** — `ApplicationDefinition` & `fromParameter` in templated Applications
- **KEP-2.16** — `SourceDefinition` authoring model; `fromSource` semantics and caching
- **KEP-2.17** — Component exports; same-topology `fromDependency`
- **KEP-2.19** — Cross-topology `fromDependency` with named topology groups