# KEP-2.22: Multi-Instance Addons

**Status:** Drafting (Not ready for consumption)
**Parent:** [vNext Roadmap](../README.md)
**Depends on:** [KEP-2.13](../2.13-addons/README.md), [KEP-2.20](../2.20-module-versioning/README.md)

## Problem

Some addons need to be installed multiple times on the same cluster with different parameters — for example, a `tenant-infra` addon that provisions per-tenant infrastructure, or an `aws-s3` addon installed independently for different teams with different region or access configurations.

The single-instance model in KEP-2.13 does not support this: the Addon CR is named after the addon, so a second install with different parameters overwrites the first. There is also no model for where definitions installed by multiple instances should live — installing the same `aws-s3-v1-bucket` definition twice from different instances with different parameters would cause each instance to stomp the other on every reconcile.

## Goals

- Enable multiple independent instances of the same addon to coexist on a cluster
- Define where definitions installed by multi-instance addons are scoped (cluster-wide vs namespace-scoped)
- Specify the Addon CR naming convention for multi-instance addons
- Enforce that definition templates do not reference instance parameters (preventing cross-instance stomping)
- Maintain full backwards compatibility — existing single-instance addons are unaffected

## `instance` — Multi-Instance Addons

The optional `instance` field in `_module.cue` is a CUE expression that, when present, marks the addon as multi-instance and determines the Addon CR name suffix. It is evaluated against the injected context — including `parameter` (the addon's `spec.parameters`):

```cue
// modules/tenant-infra/_module.cue
module:   "tenant-infra"
type:     "cue"
instance: parameter.tenant   // Addon CR name = "tenant-infra-{parameter.tenant}"
```

The resulting Addon CR name is `{addon-name}-{instance}`. The owned Application created in `vela-system` follows the `addon-` prefix convention: `addon-{addon-name}-{instance}` (e.g. `addon-tenant-infra-acme`). The instance value must be a non-empty string after evaluation; if it evaluates to `_|_` or empty, the controller surfaces a validation error and transitions the Addon CR to `Failed`.

Because `instance` is a full CUE expression, compound discriminators are natural:

```cue
// One Addon CR per tenant per region:
// Addon CR name        → "tenant-infra-acme-us-east-1"
// Owned Application    → "addon-tenant-infra-acme-us-east-1"
instance: "\(parameter.tenant)-\(parameter.region)"
```

The admission webhook validates that parameters referenced by `instance` are present in `spec.parameters` at install time. Addon authors should document which parameters are required for instance key construction.

When `instance` is absent, the addon is single-instance: the Addon CR name equals the addon name, and installing a second instance with different parameters updates the existing CR rather than creating a new one.

## Definition Scoping

Multi-instance addons cannot install definitions into `vela-system`. Definitions installed by a multi-instance addon are scoped to a namespace derived from the instance — preventing cluster-wide collision between instances that would otherwise stomp each other's definitions on every reconcile.

| Addon type | Definition target namespace | Visibility |
|---|---|---|
| Single-instance (`instance` absent) | `vela-system` | Cluster-wide |
| Multi-instance (`instance` present) | Instance namespace (derived from parameters) | Namespace-scoped |

Type resolution for Applications checks the Application's own namespace first, then falls back to `vela-system` — consistent with KubeVela's existing namespace-scoped definition resolution.

### Parameter Constraint

Definition CUE templates in multi-instance addons must not reference `parameter.*`. Parameters are valid only in control expressions — `enabled`, `instance`, and `source`. If a definition template referenced instance parameters, two instances with different parameters would produce conflicting definition bodies. The admission webhook validates this constraint on addon source tree upload.

## Addon CR Naming in Addon-of-Addons Composition

When using the `addon` component type in an Application, multi-instance addons are identified by supplying the parameters that form the instance key:

```yaml
components:
  - name: s3-team-a
    type: addon
    properties:
      name: aws-s3
      parameters:
        tenant: team-a     # → Addon CR: "aws-s3-team-a"

  - name: s3-team-b
    type: addon
    properties:
      name: aws-s3
      parameters:
        tenant: team-b     # → Addon CR: "aws-s3-team-b"
```

The OAM component name (`s3-team-a`, `s3-team-b`) is only used within the Application for dependency ordering. The Addon CR name is derived entirely from the addon's `instance` expression and the supplied parameters.

## Security Considerations

- **Instance isolation**: Multi-instance Addon CRs carry the addon registry name as their `addon.oam.dev/name` label, keeping fleet-level queries (e.g. "all installs of aws-s3") accurate regardless of per-instance CR names.
- **Namespace boundary enforcement**: The admission webhook rejects any multi-instance addon that attempts to install definitions into `vela-system`.

## Cross-KEP References

- **KEP-2.13** — Declarative addon lifecycle; Addon CR; `addon` component type
- **KEP-2.20** — Module identity; `instance` field reserved in `_module.cue`
