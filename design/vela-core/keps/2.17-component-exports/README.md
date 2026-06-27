# KEP-2.17: Component Exports & Same-Topology fromDependency

**Status:** Drafting (Not ready for consumption)
**Parent:** [vNext Roadmap](../README.md)

Intra-application data binding is today handled via workflow steps and manual data passing — exposing orchestration complexity for what should be a declarative property reference. `exports` and `fromDependency` introduce a clean, contract-driven binding model.

Cross-topology `fromDependency` (producer and consumer on different clusters) is covered in [KEP-2.19](../2.19-cross-topology-deps/README.md).

## exports in ComponentDefinition

`exports` is declared in the `ComponentDefinition` template and defines the public contract of data this component makes available to other components in the same Application. It is the only surface `fromDependency` may read from — consumers cannot access internal fields, status internals, or properties directly.

```cue
// template.cue
template: {
  output: {
    apiVersion: "apps/v1"
    kind:       "Deployment"
    // ...
  }

  // exports declares the public contract.
  // Values are CUE expressions resolved after the component reaches healthy status.
  exports: {
    endpoint:    context.status.loadBalancer.ingress[0].hostname
    port:        context.parameters.port
    serviceName: context.name
  }

  parameter: {
    port:  *80 | int
    image: string
  }
}
```

`exports` values are CUE expressions with access to `context.status`, `context.parameters`, `context.name`, and `context.outputs`. They are evaluated spoke-side after the component reaches healthy status and written to `Component.status.exports.*`. This is the same `outputs:` mechanism already defined in the Component status model — `exports` in the Definition is the authoring-time declaration of what will be written there.

## fromDependency

`fromDependency` supports two forms for same-topology dependencies (producer and consumer on the same cluster):

**Shorthand** — `<component>.<path>`:

```yaml
components:
  - name: database
    type: postgres
    properties:
      storage: 20Gi

  - name: api
    type: webservice
    properties:
      dbHost:
        fromDependency: database.endpoint    # <component>.<path>
      dbPort:
        fromDependency: database.port
      image: myapp:v1.2
```

**Map form** — required when a `default` is needed:

```yaml
      dbHost:
        fromDependency:
          component: database
          path: endpoint
          default: "localhost"
```

`fromDependency` creates an implicit ordering dependency — the Application controller will not render or dispatch `api` until `database` is healthy and its `exports` are available. No explicit `dependsOn` is required.

## Dependency Graph

The Application controller builds a dependency graph from all `fromDependency` references at render time. Cycles are rejected at admission with a clear error identifying the cycle. The graph is acyclic by enforcement — no runtime cycle detection is required.

## Phase 1 Scope

Phase 1 is limited to render-time exports only:

- `exports` values are evaluated after the component reaches healthy status
- `fromDependency` consumers wait for the referenced component's exports to be available before rendering
- No staged reconciliation or runtime dependency re-evaluation

Future phases may introduce runtime exports (re-evaluated on status change) and staged reconciliation (partial re-render when an upstream export changes).

## Security

- `exports` are declared only in `ComponentDefinition` definitions — application manifests cannot declare arbitrary exports or override what a Definition exposes
- `fromDependency` cannot traverse beyond the declared `exports` contract — no access to `properties`, raw `status`, or secrets not explicitly exported
- Secret values in exports should be declared `sensitive: true`; the component-controller redacts them from logs and limits their appearance in status

## Non-Goals

- Cross-application dependencies
- Arbitrary status field access via `fromDependency`
- Runtime dependency orchestration (Phase 1)
- Cross-topology `fromDependency` — covered in [KEP-2.19](../2.19-cross-topology-deps/README.md)