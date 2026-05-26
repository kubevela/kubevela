# KEP-2.4: Dispatcher Implementations

**Status:** Drafting (Not ready for consumption)
**Parent:** [vNext Roadmap](../README.md)

A `Dispatcher` is the pluggable delivery mechanism the hub application-controller uses to place `Component` CRs onto target clusters. Each Dispatcher implementation handles a different cluster connectivity model. Dispatchers are defined as `DispatcherDefinition` CRDs containing CUE templates — no Go recompilation is needed to add or customise a dispatcher.

## DispatcherDefinition CRD

Each dispatcher is a `DispatcherDefinition` custom resource. This follows the same model as `ComponentDefinition` and `TraitDefinition`: the behaviour is expressed entirely in CUE and evaluated at runtime by the hub controller.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: DispatcherDefinition
metadata:
  name: cluster-gateway
spec:
  # CUE template evaluated when dispatching a Component to a target cluster.
  # Context variables injected by the hub controller:
  #   context.component  – the Component object being dispatched
  #   context.target     – ClusterTarget {clusterName, namespace}
  #   context.operation  – "dispatch" | "delete"
  dispatchTemplate: |
    import "encoding/json"

    context: {
      component: {...}
      target: {
        clusterName: string
        namespace:   string
      }
      operation: "dispatch" | "delete"
    }

    // output is the ManifestWork / proxy request the controller applies
    output: {
      apiVersion: "cluster.example.io/v1alpha1"
      kind:       "ProxyRequest"
      metadata: {
        name:      context.component.metadata.name
        namespace: context.target.namespace
      }
      spec: {
        clusterName: context.target.clusterName
        manifest:    context.component
        operation:   context.operation
      }
    }

  # CUE template evaluated to resolve a topology Config into []ClusterTarget.
  # Context variables:
  #   context.config – the resolved Config object (spec.properties)
  topologyResolveTemplate: |
    context: {
      config: {
        clusterSelector?: {
          labels?: [string]: string
          names?:  [...string]
        }
        namespace?: string
      }
    }

    // targets is the list of ClusterTargets this topology resolves to.
    // The hub controller evaluates this CUE and reads the targets field.
    targets: [...{
      clusterName: string
      namespace:   string
    }]

  # Name of the ConfigTemplate this dispatcher registers for topology authoring.
  topologyConfigTemplate: cluster-gateway-topology
```

The hub controller loads the active `DispatcherDefinition`, evaluates `dispatchTemplate` (passing `context.component`, `context.target`, and `context.operation`) and applies the resulting `output` object. For topology resolution it evaluates `topologyResolveTemplate` (passing the resolved `Config` properties) and reads the resulting `targets` list.

`topologyResolveTemplate` is the critical addition — it allows the hub dependency resolution logic to be dispatcher-agnostic. The hub builds the `fromDependency` resolution graph, then delegates cluster membership queries by evaluating the active DispatcherDefinition's resolve template against each named Config.

## Topology Schema Registration

Each Dispatcher ships a `topology.cue` file alongside its Definition files. On install, the KubeVela operator registers this schema as a `ConfigTemplate` CRD. Platform engineers then create named `Config` instances using this template.

```
cluster-gateway-dispatcher/
  metadata.cue          # type: "dispatcher", name: "cluster-gateway"
  template.cue          # dispatch logic (cluster-gateway API calls)
  topology.cue          # topology schema → registered as ConfigTemplate on install
```

**cluster-gateway topology schema:**

```cue
// topology.cue — cluster-gateway-dispatcher
metadata: {
  name:        "cluster-gateway-topology"
  description: "Topology group for cluster-gateway dispatcher"
  scope:       "system"
}

template: {
  parameter: {
    // Select clusters by label selectors
    clusterSelector?: {
      labels?: [string]: string
      names?:  [...string]
    }
    // Optional namespace override for dispatched Components
    namespace?: string
  }
}
```

**OCM topology schema:**

```cue
// topology.cue — ocm-dispatcher
metadata: {
  name:        "ocm-topology"
  description: "Topology group for OCM dispatcher"
  scope:       "system"
}

template: {
  parameter: {
    // Select ManagedClusters by label selectors
    managedClusterSelector?: {
      labels?:      [string]: string
      clusterSets?: [...string]
    }
    // OCM placement tolerations
    tolerations?: [...{
      key:      string
      operator: *"Equal" | "Exists"
      value?:   string
    }]
  }
}
```

**Local topology schema:**

```cue
// topology.cue — local-dispatcher
metadata: {
  name:        "local-topology"
  description: "Topology group for local (single-cluster) dispatcher"
  scope:       "system"
}

template: {
  parameter: {
    // Namespace to deploy into on the local cluster
    namespace?: string
  }
}
```

The ConfigTemplate name (`cluster-gateway-topology`, `ocm-topology`, `local-topology`) is what the `Config` object references in `spec.template`. When a team migrates from cluster-gateway to OCM, they replace their `Config` objects' `spec.template` value and update the `properties` to match the new schema — Application specs and `fromDependency` references are unchanged.

## Built-in Dispatcher Implementations

| Dispatcher | Mechanism | Topology resolution |
|---|---|---|
| `local` | Direct API server write (same cluster) | Namespace selector only |
| `cluster-gateway` | cluster-gateway proxy API | Label/name selector against registered clusters |
| `ocm` | OCM `ManifestWork` API | `ManagedCluster` label selector + ClusterSets |

## Dispatcher Selection

The active Dispatcher for a given Application is determined by the `KubeVela` operator CR:

```yaml
spec:
  dispatcher:
    type: cluster-gateway    # or: local, ocm
```

A per-Application override is possible via annotation for migration purposes:

```yaml
metadata:
  annotations:
    app.oam.dev/dispatcher: ocm
```

## Relationship to KEP-2.17 (fromDependency)

The `ResolveTopologyGroup` method is the contract that makes cross-topology `fromDependency` dispatcher-agnostic. The hub dependency resolution loop calls `ResolveTopologyGroup(groupName)` to find which clusters a named Config maps to, then waits for `Component.status.exports` from those clusters before dispatching downstream components. The dispatcher owns the resolution logic; the hub owns the ordering.

## Relationship to KEP-2.18 (ConfigTemplate & Config CRDs)

Topology ConfigTemplates are registered by the Dispatcher on install — they are a special class of ConfigTemplate owned by the dispatcher, not hand-authored by platform engineers. The KubeVela operator ensures the topology ConfigTemplate exists before the Dispatcher begins processing Applications. If the ConfigTemplate is missing (e.g., dispatcher was uninstalled), the operator surfaces a degraded condition on the `KubeVela` CR.
