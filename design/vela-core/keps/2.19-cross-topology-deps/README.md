# KEP-2.19: Cross-Topology fromDependency

**Status:** Drafting (Not ready for consumption)
**Parent:** [vNext Roadmap](../README.md)
**Depends on:** [KEP-2.17](../2.17-component-exports/README.md), [KEP-2.4](../2.4-dispatchers/README.md), [KEP-2.18](../2.18-config-crds/README.md)

Same-topology `fromDependency` — where producer and consumer deploy to the same cluster — is covered in [KEP-2.17](../2.17-component-exports/README.md). This KEP extends that model to cross-topology dependencies, where the consuming component and the producing component are dispatched to different clusters.

## Problem

Same-topology `fromDependency` resolves spoke-locally — the spoke knows its own identity and can read the producer's `Component.status.exports` directly. When the consumer is on cluster C and the producer is on cluster A, hub-mediated resolution is required.

The hub cannot resolve `component: infra` unambiguously when `infra` is deployed to multiple clusters — it needs to know **which instance** to read from. Named topology groups solve this. A topology group is a named `Config` (KEP-2.18) of a dispatcher-registered `topology` ConfigTemplate (KEP-2.4). It binds a logical name to a cluster selector and makes the reference unambiguous.

## Cross-Topology fromDependency Syntax

Cross-topology references extend the same-topology syntax with a `/<group>` segment.

**Shorthand** — `<component>/<group>.<path>`. `/` separates component+group from the path; `.` separates path segments:

```yaml
      vpcId:
        fromDependency: infra/infra-a.vpcId   # <component>/<group>.<path>
```

**Map form** — required when a `default` is needed:

```yaml
      vpcId:
        fromDependency:
          component: infra
          group: infra-a
          path: vpcId
          default: "vpc-unknown"
```

The `group:` field is only valid in cross-topology references. Same-topology `fromDependency` (without `group:`) is covered in KEP-2.17.

## Named Topology Groups

A topology group is a named `Config` object (KEP-2.18) whose template is the dispatcher-registered topology ConfigTemplate (KEP-2.4). It binds a logical name to a cluster selector:

```yaml
apiVersion: config.oam.dev/v1beta1
kind: Config
metadata:
  name: infra-a
  labels:
    config.oam.dev/type: topology-group
spec:
  template: cluster-gateway-topology   # registered by the cluster-gateway dispatcher
  properties:
    clusterSelector:
      labels:
        region: us-east
        role: infra
```

Once topology groups are named, `fromDependency` can reference them explicitly:

```yaml
components:
  - name: api
    type: webservice
    properties:
      vpcId:
        fromDependency:
          component: infra
          group: infra-a          # named topology group → unambiguous cluster target
          path: vpcId
```

The topology policy in the Application references named groups instead of inline selectors:

```yaml
policies:
  - name: spread-infra
    type: topology
    properties:
      groups: [infra-a, infra-b]
```

Inline selectors remain valid for Applications that only use same-topology dependencies. Named groups are only required when cross-topology `fromDependency` is used.

## Hub Resolution

The hub resolves a cross-topology `fromDependency` as follows:

1. Look up the named `Config` (e.g. `infra-a`) → call `ResolveTopologyGroup(groupName)` on the active Dispatcher (KEP-2.4) to get the cluster list
2. Find the `Component` CR for the named component dispatched to that cluster
3. Read `Component.status.exports.<path>`
4. Substitute the concrete value into the consumer's properties before dispatch

The hub does not dispatch the downstream component until the upstream export is available. This creates the same implicit ordering dependency as same-topology `fromDependency` (KEP-2.17).

For OCM, exports are declared as ManifestWork feedback rules on `Component.status.exports.*` fields. The hub reads them from ManifestWork status.

## Admission Validation

Cross-topology `fromDependency` is validated at admission time using the same type-checking mechanism as same-topology (KEP-2.17):

- The `group` value must reference a `Config` object with `config.oam.dev/type: topology-group` in the same namespace or in `vela-system`
- The `path` must refer to a field declared in the producer `ComponentDefinition`'s `exports{}` schema
- The type of the export field must be compatible with the consumer component's `parameter{}` schema at the given property path

## Migration: Inline to Named Topology Groups

Teams that want to use cross-topology `fromDependency` must migrate their topology policies from inline selectors to named `Config` references. This is opt-in — inline selectors continue to work for Applications that only use same-topology dependencies.

```bash
# Extract inline topology selectors into named Config objects
vela migrate topology --namespace production --dry-run
vela migrate topology --namespace production
```

The command creates a `Config` per unique inline selector found, assigns a generated name (e.g. `topology-us-east-infra`), and rewrites the Application policy to reference it by name.

## Non-Goals

- Cross-application dependencies
- Same-topology `fromDependency` — covered in [KEP-2.17](../2.17-component-exports/README.md)
- Cross-topology `fromDependency` using inline selectors — named topology groups are required
- Runtime dependency re-evaluation on upstream status change (Phase 1)

## Cross-KEP References

- **KEP-2.17** — Component exports & same-topology `fromDependency`; this KEP extends that model
- **KEP-2.4** — Dispatcher `ResolveTopologyGroup` method; topology ConfigTemplate registration per dispatcher type
- **KEP-2.18** — `Config` CRD used for named topology group objects
- **KEP-2.3** — Hub reconcile pipeline; hub-mediated resolution and dispatch ordering