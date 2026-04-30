# KEP-2.8: KubeVela v1 → v2 Migration

**Status:** Drafting (Not ready for consumption)
**Parent:** [vNext Roadmap](../README.md)

Migration is the highest community-adoption risk for the vNext Roadmap. The goal is a **zero-downtime, opt-in, incremental migration path** — teams can adopt v2 piece-by-piece over months, existing v1 Applications keep running without modification, and the cluster operator is never forced into a big-bang cutover.

## Guiding Principles

1. **Additive CRD changes first.** New fields on existing CRDs (`Application`, `ComponentDefinition`, `TraitDefinition`) are added as optional fields. v1 controllers ignore unknown fields; v2 controllers read them. No CRD removal until after a two-release deprecation window.
2. **Dual-stack coexistence.** v1 and v2 controllers run side-by-side, gated by a feature flag on the `KubeVela` operator CR. The hub application-controller gains a `spec.controllerMode` field:
   - `v1` (default at upgrade) — existing behaviour, no spoke dispatch.
   - `dual` — v1 behaviour for unlabelled Applications; v2 spoke dispatch for Applications labelled `app.oam.dev/controller-mode: v2`.
   - `v2` — all Applications use v2 spoke dispatch; v1 path is disabled.
3. **ResourceTracker natural lifecycle.** Hub-side ResourceTrackers (v1) are not forcibly migrated. They live out their natural lifecycle — the last Application reconcile that completes GC removes them. New Applications provisioned after mode switch get spoke-side ResourceTrackers immediately. Existing Applications are migrated lazily on the first v2 reconcile.
4. **Conversion webhooks for API version.** `ComponentDefinition` and related Definition CRDs will be served at both `core.oam.dev/v1beta1` and `core.oam.dev/v2alpha1`. A conversion webhook translates between the two on read/write so existing tooling (`kubectl`, CI, VelaCLI) continues to work against v1beta1 without change.

## Phase 1 — Additive CRD Extensions (no behaviour change)

Ship these in a minor v1 release before v2 goes GA:

| Field | CRD | Purpose |
|---|---|---|
| `spec.controllerMode` | `KubeVela` | Gate per-Application v2 dispatch |
| `metadata.labels["app.oam.dev/controller-mode"]` | `Application` | Opt an Application into v2 |
| `spec.component.definitionRef.apiVersion` | `Component` | Track which Definition API version was used at dispatch |
| `spec.definitionSnapshot.inline` | `Component` | Encrypted Definition snapshot for spoke-side rendering (KEP-2.5) |
| `status.workflow` | `Component` | Spoke workflow state persistence (KEP-2.2) |

All fields are optional with zero-value defaults that preserve v1 semantics. A cluster running this release is fully v1 — no operational change.

## Phase 2 — Spoke Controller Deployment

The spoke `component-controller` is deployed as a new `Deployment` managed by the `KubeVela` operator. It does not interfere with the hub application-controller. The operator CR controls which spokes get the controller:

```yaml
apiVersion: install.oam.dev/v1beta1
kind: KubeVela
spec:
  controllerMode: dual           # hub stays in v1 mode for unlabelled Apps
  spokeController:
    enabled: true
    clusters: ["*"]              # deploy to all registered clusters
    image: oamdev/vela-core:v2.0.0
```

During `dual` mode:
- Applications **without** `app.oam.dev/controller-mode: v2` are reconciled by the existing hub application-controller path (hub ResourceTracker, hub-side trait injection, existing workflow engine).
- Applications **with** `app.oam.dev/controller-mode: v2` are reconciled by the new hub path: parse → snapshot → dispatch `Component` CR to spoke → watch `Component.status.health`.

Teams can opt individual Applications in by adding the label. This allows progressive rollout within a cluster without disrupting other teams.

## Phase 3 — Definition Migration

`ComponentDefinition` and `TraitDefinition` CRDs gain a `core.oam.dev/v2alpha1` version served via conversion webhook. The conversion is mostly a no-op for v1-compatible Definitions — field names are unchanged, and the workflow block is new-optional (Definitions without `workflow:{}` use the implicit dispatch-only path).

Definitions that use v2-only features (`workflow: create:{}`, `fromSource:`, `fromDependency:`) must be authored against `v2alpha1`. The conversion webhook rejects a downgrade request for such Definitions with a clear error:

```
ComponentDefinition "helm-chart" cannot be converted to v1beta1:
  field workflow.create is not representable in v1beta1.
  Remove v2-only fields before downgrading.
```

**Definition migration checklist for platform teams:**

- [ ] Audit existing `ComponentDefinition` CUE templates for deprecated patterns (e.g., inline workflow steps that should move to `workflow: create:{}`).
- [ ] Add `workflow: create: [{ type: "dispatch" }]` to Definitions that need explicit dispatch control — no-op functionally but makes the lifecycle explicit.
- [ ] Replace ad-hoc Helm steps with `workflow: create: [{type: "helm-install"}, {type: "dispatch"}]` pattern.
- [ ] Test each Definition with `vela def dry-run --mode v2` before switching the Application label.

## Phase 4 — ResourceTracker Migration

When an Application is switched to v2 mode, its hub-side ResourceTracker is handled as follows:

1. **On the first v2 reconcile**, the hub application-controller detects a hub-side `currentRT` for this Application.
2. It annotates the `currentRT` with `resourcetracker.oam.dev/migration-policy: preserve-until-gc`. This prevents the v2 path from also creating a hub-side RT.
3. The v2 path dispatches the `Component` CR to the spoke. The spoke creates its own spoke-side ResourceTracker.
4. On the **next GC cycle** for the Application (triggered by a delete or a deploy that removes components), the hub-side RT's managed resources are either:
   - Already tracked by the spoke RT (overlap) → hub RT simply removes the reference; spoke owns the resource.
   - Not tracked by the spoke RT (gap, e.g., resources removed before v2 migration) → normal GC applies.
5. Once the hub RT's `Spec.ManagedResources` list reaches zero, it is deleted normally.

This coexistence means no resources are double-deleted and no manual state transfer is needed.

## Phase 5 — Mode Promotion

Once all Applications in a cluster have been migrated to v2 mode (or decommissioned), the operator CR `spec.controllerMode` can be changed to `v2`. This:

- Disables the v1 application-controller reconcile path.
- Rejects `Application` resources with `app.oam.dev/controller-mode: v1` (admission webhook warning, not hard block — gives teams time to clean up labels).
- Removes the dual-mode overhead from the hub controller.

The `KubeVela` operator exposes a migration progress status:

```yaml
status:
  migration:
    v1Applications: 3          # still on v1 path
    v2Applications: 47         # already migrated
    hubResourceTrackers: 2     # remaining hub-side RTs pending natural GC
    readyForModePromotion: false
```

## Application Spec Changes — Backward Compatibility

The Application `spec.components[].type`, `spec.components[].properties`, and `spec.policies[]` fields are unchanged between v1 and v2 — these are the fields most application teams interact with. The migration is transparent to application authors.

The one user-visible change is the removal of `spec.components[].traits[].properties` inline trait injection in favour of policy-expressed traits (KEP-2.9). This is handled by the conversion webhook: the webhook detects inline traits, promotes them to the `spec.policies[]` list as `TraitPolicy` entries, and annotates the Application with `app.oam.dev/migrated-traits: "true"` so operators can audit the promotion.

## `vela migrate` CLI Tooling

A `vela migrate` subcommand assists cluster operators:

```bash
# Dry-run: show what would change
vela migrate plan --namespace production

# Opt a namespace into v2 mode
vela migrate apply --namespace production --mode v2

# Report migration status across all namespaces
vela migrate status

# Roll back a namespace to v1 mode
vela migrate rollback --namespace production
```

`vela migrate plan` outputs a diff table:

```
NAMESPACE    APPLICATION      CURRENT MODE   PROPOSED MODE   ISSUES
production   checkout-api     v1             v2              none
production   payment-svc      v1             v2              inline trait: "gateway" → will be promoted to policy
staging      infra-setup      v1             v2              hub RT has 12 managed resources (will migrate lazily)
```

## Open Questions

- **Inline trait promotion semantics**: When inline `traits[]` are promoted to `TraitPolicy` entries by the conversion webhook, the policy's selector must match the source component exactly. Confirm that the TraitPolicy selector model (KEP-2.9) supports single-component targeting without ambiguity.
- **Hub RT migration timeout**: Should the operator enforce a maximum coexistence window (e.g., 30 days) after which hub RTs are force-deleted? Or leave it entirely to natural lifecycle? Force-deletion risks leaving orphaned resources if spoke RT is incomplete.
- **Multi-tenancy**: In a cluster with many namespaces managed by different teams, `vela migrate apply` must respect RBAC. The CLI should use the calling user's credentials, not a cluster-admin token.
