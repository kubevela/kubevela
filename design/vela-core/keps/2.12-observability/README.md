# KEP-2.12: ObservabilityDefinition

**Status:** Drafting (Not ready for consumption)
**Parent:** [vNext Roadmap](../README.md)

A standalone CRD for fleet-level observability across Component instances. Decoupled from `ComponentDefinition` so teams can publish, update, and scope observability views independently.

```yaml
apiVersion: core.oam.dev/v2alpha1
kind: ObservabilityDefinition
metadata:
  name: postgres-fleet-health
spec:
  template: |
    // Can target one or multiple Definition types
    targets: [
      { type: "postgres-database" },
    ]

    views: [
      {
        name: "fleet-health"
        interval: "60s"
        selector: {
          namespaces: ["production", "staging"]
          labels: { "team": "platform" }
        }
        metrics: [
          {
            name: "healthy_ratio"
            type: "gauge"
            value: len([ for i in context.instances if i.status.isHealthy { i } ]) / len(context.instances)
          }
        ]
        alerts: [
          {
            name: "HighUnhealthyRatio"
            condition: healthy_ratio < 0.8
            severity: "critical"
            message: "Fewer than 80% of production instances are healthy"
          }
        ]
      },
      {
        name: "version-drift"
        interval: "10m"
        selector: {
          fields: ["parameters.version"]   // only fetch version field — minimal overhead
        }
        logs: [
          {
            condition: len({ for i in context.instances { i.parameters.version: true } }) > 1
            level: "warning"
            message: "Multiple versions running across fleet"
            fields: {
              versions: [ for i in context.instances { i.parameters.version } ]
            }
          }
        ]
      }
    ]
```

## Key Design Properties

- **Views** are independently scoped — each declares its own `selector`, `interval`, and outputs
- **`selector.fields`** limits what the controller fetches per instance — no surprise expensive queries
- **Multiple targets** — a single `ObservabilityDefinition` can query across `postgres-database`, `redis`, `kafka` etc. for holistic views (e.g. `data-tier-overview`)
- **Multiple teams** can publish different `ObservabilityDefinition` objects over the same Definition type — infra team watches storage, security team watches version drift, app team watches latency
- **Outputs**: `metrics` (Prometheus), `logs` (structured stdout), `alerts` (Kubernetes Events)

## Future Enhancement: Per-Component Observability

`context.status` is already the right hook for golden signals. A CUE-configurable `observability` block in the Definition that the component-controller uses to auto-register metrics with Prometheus — no per-Definition wiring required:

```cue
// observability.cue
observability: {
  metrics: [
    {
      name: "ready_replicas"
      help: "Number of ready replicas"
      type: "gauge"
      value: context.status.database.readyReplicas
    },
    {
      name: "storage_bytes"
      help: "Allocated storage in bytes"
      type: "gauge"
      value: context.parameters.storage  // parsed from "10Gi" → bytes
    }
  ]
}
```

The component-controller exposes a `/metrics` endpoint per Component instance. Platform teams get golden signals for free — no Prometheus exporters to write, no ServiceMonitors to configure per Definition.
