# Design 04: Cluster Scope

**Status:** Proposed, and deliberately secondary. It is not the case [KEP-2.15](../README.md) argues for, and nothing in the KEP depends on it. It is additive: a third value on an existing enum, sharing machinery that already exists.

**Companion to:** [KEP-2.15](../README.md), in particular [Attachment](../README.md#attachment) and [Execution Model](../README.md#execution-model).

> **TL;DR**
> - A third scope for operations whose subject is the cluster, not a Component or an Application.
> - It exists because `WorkflowRun` cannot serve it: spoke-local and single-cluster, with no fleet view to report back to.
> - The context shrinks to the cluster and operation-level keys, `#AppIdentity` and `#ComponentIdentity` both dropping away. That is the point, not a shortfall.
> - It needs no permission model of its own. The KEP's three gates apply, with the target gate and the cluster gate collapsing into one check.
> - The fan-out is map-only. Fleet-wide *aggregation* needs machinery this design does not propose.

## The case

A procedure whose subject is a cluster has nowhere to live today.

Read `cluster-info` from every spoke and record what is found. Check that a bootstrap prerequisite is present across the fleet. Collect a version or a capability flag from each member cluster. None of these has a Component or an Application to attach to, and all of them are hub-orchestrated fan-out.

`WorkflowRun` is the obvious home and cannot be it. It is an optional addon running a per-instance controller on the spoke, so it is single-cluster by construction and has no cluster-gateway path back to the hub. A runbook that reads on N spokes and writes once on the hub is asymmetric in exactly the way a spoke-local controller cannot express.

That asymmetry is what makes this an `Operation` rather than a gap in `WorkflowRun`. The hub orchestration, the per-cluster fan-out, the identity model and the run-to-completion lifecycle already exist here.

**It is not attachment switched off.** The target is the cluster. That is why it is a scope with its own semantics rather than a flag meaning "no target".

## Shape

```yaml
apiVersion: core.oam.dev/v2alpha1
kind: OperationTemplate
metadata: {name: sync-cluster-info, namespace: vela-system}
spec:
  attach:
    scope: Cluster
    includeHub: false               # default. true lets local be named in spec.clusters
    clusterSelector:
      matchLabels:
        vela.io/managed: "true"
  runAs:
    mode: Platform
    serviceAccountName: op-cluster-reader
  workflow:
    steps:
      - name: read-cluster-info
        type: read-object
        properties:
          apiVersion: v1
          kind: ConfigMap
          name: cluster-info
          namespace: kube-system
          cluster: context.cluster          # the spoke this workflow is running against
        outputs:
          - {name: info, valueFrom: output.value.data}

      - name: record-on-hub
        type: apply-object
        inputs:
          - {from: info, parameterKey: value.data}
        properties:
          cluster: local                    # explicit, or it inherits the spoke
          value:
            apiVersion: v1
            kind: ConfigMap
            metadata:
              name: "cluster-info-\(context.cluster)"
              namespace: vela-system
```

The invocation, whose only visible difference is the absence of a target:

```yaml
kind: Operation
metadata: {name: sync-info-20260821, namespace: vela-system}
spec:
  template: sync-cluster-info
  clusters: [eu-west-1, eu-central-1, us-east-1]
```

**`spec.clusters` is required under this scope.** For Component scope the clusters can be derived from where the component is deployed. With no target there is no other source, and defaulting a template that can do anything to "every cluster" is not a default worth shipping.

`clusterSelector` bounds what may be named. Admission resolves it and refuses an `Operation` naming a cluster outside the result, the same way it refuses a Component-scoped operation whose target is the wrong type.

## Why `includeHub` rather than relying on labels

`NewVirtualClusterFromLocal()` (`pkg/multicluster/virtual_cluster.go`) synthesises the hub with an empty label map, so the hub can never match a `matchLabels` selector. Fleet-wide templates therefore exclude it for free.

That is accidental rather than designed, and it stops being true the moment the hub gains labels, which is what [KEP-2.20 Design 04](../../2.20-module-versioning/design/04-cluster-context.md) wants. A behaviour worth relying on should not rest on a map being empty, so hub inclusion is stated rather than inferred.

## Context

The [context table](../README.md#cue-context) in the KEP carries the scope column, so it is not restated here. Under this scope the surface is `#ClusterIdentity`, `#StepIdentity` and `#OperationIdentity`: everything marked *all three*, and nothing marked *Component* or *Component, Application*.

In practice that is `context.cluster`, `context.clusterVersion`, `context.namespace`, `context.stepName` and the `context.operation*` keys. Three consequences.

**Hub-versus-spoke branching needs no new key.** `if: context.cluster == "local"` is already the idiom the KEP uses, so it comes free.

**`context.clusterVersion` is more useful here than anywhere else.** It exists today and is available in every scope, but a fleet-wide procedure branching on a cluster's Kubernetes version is the case it was made for.

**Nothing else cluster-descriptive is available.** No labels, no provider, no region. A template wanting those reads them out of the cluster itself, which is what the example above does. Selection by label happens controller-side at admission, against the `VirtualCluster`; exposing labels at step time is [KEP-2.20 Design 04](../../2.20-module-versioning/design/04-cluster-context.md)'s to grant and is not assumed here.

## Permissions

Nothing scope-specific is needed, which is the point. [KEP-2.15's three gates](../README.md#may-the-invoker-act-on-the-target-there) apply unchanged:

| | Component / Application | Cluster |
|---|---|---|
| act on the target | `operate` on the `Application` | the cluster *is* the target |
| run the procedure | `invoke` on the `OperationTemplate` | unchanged |
| run it there | `operate` on each `VirtualCluster` | the same check, now doing both jobs |
| coarse gate | `create` on `operations` in the namespace | unchanged |

With no Application, the target gate and the cluster gate are the same question asked once: may this person operate on `eu-west-1`. So a scope that looked like it needed a bespoke permission model needs none, and the count stays at two distinct checks rather than dropping to one.

```yaml
kind: ClusterRole
rules:
  - apiGroups: ["cluster.core.oam.dev"]
    resources: ["virtualclusters"]
    resourceNames: ["eu-west-1", "eu-central-1"]
    verbs: ["operate"]
```

`VirtualCluster` (`cluster.core.oam.dev/v1alpha1`, served by cluster-gateway) carries no credentials, is uniform across cluster-secret and OCM backings, and is the same object `clusterSelector` matches labels against. The `runAs` account's own RBAC inside each member cluster remains the enforcement underneath, unchanged from every other scope.

## What this does not do

**The fan-out is map-only.** One workflow per cluster, isolated, each writing its own row. There is no path for per-cluster results to reach a hub-side step: `status.workflows[].children[]` tracks phase for observability, not outputs, and parent/child composition waits on completion without reading results.

So "each cluster records its own entry" works with the machinery described here. "Collect from the fleet, then write one summary" does not. A reduce step is a real design with failure semantics of its own, principally what a summary written from eight of ten clusters means, and whether that is useful or dangerous. It is not proposed here.

## The risk worth naming

This dilutes the thesis. KEP-2.15 argues that Day 2 knowledge belongs with the component, and attachment is what makes that argument. A scope with no attachment is a different capability sharing the plumbing.

The danger is not the feature, it is that detached becomes the path of least resistance: a template author who cannot be bothered to model attachment writes `scope: Cluster` and passes the target as a parameter, and the platform sprawl this KEP set out to reduce reappears inside the mechanism meant to reduce it.

Two things hold it in check, and both should survive review. The context table is the honest signal: choosing this scope costs the author every component-facing key, so a procedure that really is about a component is worse to write this way. And `scope` is mirrored into a label, so "how many of our templates are detached" is a query rather than an audit.

## Open questions

1. **Whether the cluster gate should have shipped with the primary model all along.** It is proposed in the KEP proper rather than here, because an Application spanning environments has the same hole. If review disagrees and wants it confined to this scope, that is a smaller change than adding it later.
2. **What a cluster excluded mid-run means.** `clusterSelector` is resolved at admission, but a label can change while a long operation is suspended. Re-checking on resume is consistent with how [suspend re-reads sources](../README.md#what-a-long-pause-actually-means), and it means an operation can become invalid while it waits.
3. **Whether `includeHub` is enough**, or whether hub-targeting deserves its own scope. A procedure that runs only on the hub is not fan-out at all, and may have more in common with a `WorkflowRun` than with this.
4. **Whether aggregation belongs here later.** If it does, this design should not foreclose it; if it does not, the boundary with `WorkflowRun` needs restating a third time.
