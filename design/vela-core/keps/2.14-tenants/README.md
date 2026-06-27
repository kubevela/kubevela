# KEP-2.14: TenantDefinition & Tenant

**Status:** Drafting (Not ready for consumption)
**Parent:** [vNext Roadmap](../README.md)

Multi-tenancy is a first-class platform primitive in the vNext Roadmap. Rather than leaving namespace provisioning, RBAC wiring, quota setup, and cluster access scoping as manual operational tasks, the `TenantDefinition` and `Tenant` CRDs make tenancy a fully reconciled, version-controlled platform capability.

## TenantDefinition

`TenantDefinition` is a cluster-scoped CRD in the `core.oam.dev/v2alpha1` API version. Platform engineers define different tenancy archetypes — `team`, `environment`, `external-partner` — each with their own provisioning rules. Like all Definitions, it follows the standard CUE authoring model: a `metadata.cue` entrypoint, a `template:` block containing `parameter` and `output`, and supporting files merged via CUE unification.

```
my-tenant-definition/
  metadata.cue       # name, type: "tenant", description, attributes
  template.cue       # parameter schema + output
  rbac.cue           # RBAC output fragments
  quotas.cue         # ResourceQuota / LimitRange output fragments
  application.cue    # inline Application or ApplicationDefinition reference
```

The `output` of a `TenantDefinition` is a structured provisioning spec containing:

- **`namespaces`** — list of namespaces to create, with labels and annotations
- **`rbac`** — Groups, Roles, RoleBindings, and ClusterRoleBindings to provision on the hub; rendered as Kubernetes RBAC resources applied to the hub
- **`clusterAccess`** — label selectors over `Cluster` CRs (from the Cluster KEP) defining which spoke groups this tenant can dispatch to
- **`application`** — either an inline `Application` spec or a reference to an `ApplicationDefinition` + parameters; deployed to the hub and continuously reconciled; used to provision shared tenant infrastructure (e.g. Crossplane claims, secrets, monitoring stacks)
- **`quotas`** — `ResourceQuota` and `LimitRange` objects applied per namespace

```cue
// template.cue
template: {
  output: {
    namespaces: [
      { name: "\(context.parameters.teamName)-dev",  labels: { "tenant": context.parameters.teamName } },
      { name: "\(context.parameters.teamName)-prod", labels: { "tenant": context.parameters.teamName } },
    ]
    rbac: [
      {
        kind:     "RoleBinding"
        name:     "\(context.parameters.teamName)-developers"
        subjects: context.parameters.members
        roleRef:  "developer"
        namespaces: ["\(context.parameters.teamName)-dev", "\(context.parameters.teamName)-prod"]
      }
    ]
    clusterAccess: {
      matchLabels: { "spoke-group": "app" }
    }
    application: {
      definition: "team-infra"
      parameters: {
        teamName: context.parameters.teamName
        costCentre: context.parameters.costCentre
      }
    }
    quotas: [
      {
        namespace: "\(context.parameters.teamName)-dev"
        hard: { "requests.cpu": "4", "requests.memory": "8Gi" }
      }
    ]
  }

  parameter: {
    teamName:   string
    costCentre: string
    members: [...{ kind: string, name: string }]
  }
}
```

## Tenant

`Tenant` is a cluster-scoped CR. It references a `TenantDefinition` by name and supplies parameters. The hub's tenant-controller reconciles it continuously — provisioning and drift-correcting all declared resources.

```yaml
apiVersion: core.oam.dev/v2alpha1
kind: Tenant
metadata:
  name: team-payments
spec:
  definition: team-tenant
  # definitionRevision pins to a specific DefinitionRevision.
  # If omitted, resolves to the current revision at creation time and pins.
  # Explicit update required to adopt a new revision.
  definitionRevision: team-tenant-v2
  parameters:
    teamName: payments
    costCentre: CC-4412
    members:
      - kind: User
        name: alice@example.com
      - kind: Group
        name: payments-engineers
```

## Revision Pinning

`TenantDefinition` revisions are managed via the existing `DefinitionRevision` mechanism. When a `Tenant` CR is created, the tenant-controller resolves the current `TenantDefinition` revision and writes it into `spec.definitionRevision`. All subsequent reconciles use that pinned revision — the tenant is isolated from Definition changes until a platform engineer explicitly updates `spec.definitionRevision`. This mirrors how `Component` instances are pinned to Definition snapshots.

## Lifecycle

| Event | Controller action |
|---|---|
| `Tenant` created | Resolve Definition revision, pin it, provision namespaces + RBAC + quotas, deploy Application |
| `Tenant` updated (parameters) | Re-render against pinned revision, reconcile diff |
| `Tenant` updated (`definitionRevision`) | Re-render against new revision, reconcile diff (may add/remove namespaces, update RBAC) |
| `TenantDefinition` updated | No effect on existing Tenants — revision pinning isolates them |
| `Tenant` deleted | Finalizer runs: delete owned Application, remove RBAC, optionally retain namespaces (configurable via `spec.namespaceRetainPolicy: Retain \| Delete`) |

## Relationship to Other KEPs

- **Cluster KEP** — `clusterAccess` label selectors are evaluated against `Cluster` CRs; the Cluster KEP must expose spoke group labels for this to work
- **Definition RBAC KEP** — `TenantDefinition` access control (`access.cue`) gates which platform engineers can create which tenant types
- **ApplicationDefinition (KEP-2.9)** — the `application` block in a `TenantDefinition` output may reference an `ApplicationDefinition` by name, composing tenant provisioning with the platform's application delivery model
