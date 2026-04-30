# KEP-2.9: `ApplicationDefinition` & `PolicyDefinition`

**Status:** Drafting (Not ready for consumption)
**Parent:** [vNext Roadmap](../README.md)

## The Missing Composition Layer

KubeVela v1 makes it easy for platform engineers to publish `ComponentDefinitions`, `TraitDefinitions`, and `PolicyDefinitions` for developers to consume. But applications frequently share large amounts of configuration — the same component combinations, the same policy sets, the same trait patterns. Without a composition layer, developers either write verbose Application specs by hand or platform engineers resort to Helm charts that bypass the model entirely.

`ApplicationDefinition` fills this gap — a developer-facing composition primitive that lets platform engineers publish opinionated, parameterised application constructs built from the smaller building blocks they deliver. Similar in concept to Crossplane Compositions and KRO, but expressed in CUE and fully integrated with the KubeVela model.

## ApplicationDefinition CRD

Follows the same CUE authoring conventions as ComponentDefinition — metadata block + `template:` at the top level, `parameter` and `output` inside `template`. `output` contains a fully-formed Application spec.

```cue
// web-service.cue
"web-service": {
  type:        "application-definition"
  description: "Opinionated web service with frontend, ingress, and database"
  attributes: {}
}

template: {
  parameter: {
    image:    string
    replicas: int | *1
    domain:   string
    dbSize:   string | *"10Gi"
  }

  output: {
    components: [
      {
        name: "frontend"
        type: "deployment"
        properties: {
          image:    parameter.image
          replicas: parameter.replicas
        }
        traits: [
          { type: "ingress", properties: { domain: parameter.domain } }
          { type: "scaler",  properties: { replicas: parameter.replicas } }
        ]
      },
      {
        name: "database"
        type: "postgres-database"
        properties: {
          storage: parameter.dbSize
        }
      }
    ]
    policies: [
      { type: "garbage-collect" }
      { type: "topology", properties: { clusters: ["prod"] } }
    ]
  }
}
```

The Application CR becomes minimal — just a definition reference and parameters:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: my-app
  namespace: team-a
spec:
  definition: web-service
  parameters:
    image:    "myapp:v1.2.0"
    replicas: 3
    domain:   "myapp.example.com"
    dbSize:   "20Gi"
```

The application-controller resolves the `ApplicationDefinition`, evaluates the CUE with the provided parameters, and gets back a fully-formed Application spec. The resolved spec is then reconciled exactly as if the developer had written it by hand.

## PolicyDefinition CRD

Named, reusable policy definitions. Platform engineers publish `PolicyDefinition` objects; `ApplicationDefinitions` and `Application` CRs reference them by name.

Follows the same CUE authoring convention:

```cue
// topology-prod.cue
"topology-prod": {
  type:        "policy-definition"
  description: "Deploy to production clusters with standard placement rules"
  attributes: {}
}

template: {
  parameter: {
    clusters: [...string] | *["prod-cluster-1", "prod-cluster-2"]
  }

  output: {
    type: "topology"
    properties: {
      clusters: parameter.clusters
    }
  }
}
```

Referenced in an `ApplicationDefinition`:

```cue
policies: [
  { type: "topology-prod" }
  { type: "garbage-collect" }
]
```

## Separation of Concerns

| Role | Owns |
|---|---|
| Platform Engineer | `ComponentDefinition`, `TraitDefinition`, `WorkflowStepDefinition`, `PolicyDefinition` |
| App Platform Engineer | `ApplicationDefinition` — composes platform primitives into opinionated app constructs tailored to developer personas |
| Developer | `Application` — either references an `ApplicationDefinition` with parameters, or hand-crafts their own Application from available building blocks |

Platform engineers deliver the building blocks. App platform engineers compose them into application patterns tailored to developer personas (`web-service`, `data-pipeline`, `ml-training-job`). Developers consume the patterns with minimal configuration — or compose their own Applications directly from the named component and trait types the platform exposes. The CUE machinery is entirely invisible to developers at both levels.

## ApplicationRevision Definition Snapshot

When an `ApplicationRevision` is created, the hub resolves and snapshots all Definition definitions the Application was rendered against. This mirrors how `ComponentDefinition` revisions are captured today. The snapshot set in vNext includes:

| Definition type | Purpose |
|---|---|
| `ComponentDefinition` | Component rendering and exports schema |
| `TraitDefinition` | Trait pipeline rendering |
| `WorkflowStepDefinition` | Workflow step execution |
| `PolicyDefinition` | Policy rendering |
| `SourceDefinition` | Source resolution schema and CueX template |

Re-renders and rollbacks of a historical `ApplicationRevision` always use the snapshotted Definition versions — not whatever is currently installed in the cluster. For `SourceDefinition` this is particularly important: the `output{}` schema in the snapshot determines the `ConfigTemplate` version used for cache lookups, ensuring cache correctness across controller restarts and Definition upgrades.

## Application Parameters & fromParameter

Applications support an optional `spec.parameters` map — a free-form set of values that represent application-level constants shared across components, traits, policies, and operations. This avoids duplicating cross-cutting values (regions, tier names, cost centres) across every component's properties.

### Templated Applications

When an Application is created from an `ApplicationDefinition`, `spec.parameters` is validated against the Definition's `parameter{}` schema at admission time. The Application controller renders the Application spec from the Definition template with `parameter` values injected — component properties, trait properties, and policy properties are all CUE-rendered, so `parameter.*` references resolve naturally.

### Non-Templated Applications

Non-templated Applications also accept `spec.parameters` as a free-form map. Rather than a CUE render pass, property values may reference parameters via `fromParameter` — an explicit substitution directive that sits inline within `properties`:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
spec:
  parameters:
    region: us-west-2
    tier: production
    enableMultiAz: true

  components:
    - name: my-db
      type: aws-rds
      properties:
        # regular value — no substitution
        instanceClass: db.t3.medium
        # fromParameter resolves before Definition rendering
        region:
          fromParameter: region
        multiAz:
          fromParameter: enableMultiAz

    - name: my-cache
      type: elasticache
      properties:
        region:
          fromParameter: region
        tier:
          fromParameter: tier
```

The Application controller resolves all `fromParameter` references before handing properties to the Definition renderer. If a referenced key is absent from `spec.parameters`, the controller surfaces a validation error at reconcile time with a clear message identifying the missing key.

`fromParameter` is valid within `properties` at any depth — including nested objects and array entries. It is not valid as a map key, only as a value.

### Operations

`spec.parameters` is available in Operations via `context.appParams`. For templated Applications with a known schema, `OperationDefinition` authors may declare `requiredAppDefinition` to rely on specific keys safely. For non-templated Applications, authors should use CUE defaults (`context.appParams.region | "us-east-1"`) or declare dependencies explicitly in the Operation's own `parameter{}` schema and have callers wire them via `fromParameter` or explicit values.

`fromParameter` is not available in `OperationDefinition` templates — Operations use `context.appParams` directly.

Captured as part of `KEP-2.3: Hub application-controller integration`.
