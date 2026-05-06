# KEP-2.10: Compositions

**Status:** Drafting (Not ready for consumption)
**Parent:** [vNext Roadmap](../README.md)

Compositions are a first-class pattern for composing multiple ComponentDefinitions into higher-order abstractions. Rather than ApplicationDefinition (which targets platform engineers building developer-facing constructs), Compositions address the common case of combining smaller, well-defined Definitions into a single deployable unit with a unified parameter surface.

A Composition is itself a ComponentDefinition whose `outputs:` list includes entries of `type: component`. Each `type: component` output declares the child Component — its Definition reference, parameter mapping from the parent's `context.parameters`, and lifecycle relationship.

```cue
// database-stack composition
template: {
  outputs: [
    {
      name: "postgres"
      type: component
      definition: "postgres-definition"
      parameters: {
        storage:  context.parameters.storage
        replicas: context.parameters.replicas
      }
    },
    {
      name: "pgbouncer"
      type: component
      definition: "pgbouncer-definition"
      parameters: {
        host: context.outputs["postgres"].status.host
      }
    },
    {
      name: "metrics-exporter"
      type: component
      definition: "postgres-exporter-definition"
      parameters: {
        dsn: context.outputs["postgres"].status.connectionString
      }
    }
  ]

  parameter: {
    storage:  *"10Gi" | string
    replicas: *1 | int
  }
}
```

Child Components are instantiated on the same spoke as the parent. Their lifecycle — create, update, delete — is governed by the parent Component controller. Output chaining (`context.outputs["postgres"].status.host`) is resolved after the referenced child reaches healthy status, creating an implicit ordering dependency without requiring explicit workflow steps.

## Benefits

Compositions enable:
- **Definition reuse at scale** — platform teams build small, focused Definitions and assemble them rather than writing monolithic templates
- **Clean parameter surfaces** — parent exposes only the parameters that vary; child defaults handle the rest
- **Dependency-ordered rollout** — the component-controller respects output dependency chains when reconciling child Components
- **Independent health aggregation** — the Composition is healthy only when all child Components are healthy; individual child status surfaces up through `context.outputs`
