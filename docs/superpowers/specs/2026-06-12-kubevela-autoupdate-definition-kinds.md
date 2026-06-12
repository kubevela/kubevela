# KubeVela `app.oam.dev/autoUpdate` — which definition kinds does it cover?

## TL;DR

When an Application carries `app.oam.dev/autoUpdate: "true"`, the
controller treats a change to **any referenced definition kind** as
grounds to cut a new ApplicationRevision and re-render. That includes
ComponentDefinition, TraitDefinition, PolicyDefinition,
WorkflowStepDefinition, and WorkloadDefinition. Without the annotation,
the controller only notices changes to the App's own spec; definition
changes are ignored even when the live definition has obviously moved.

## Setup

| Item | Value |
| --- | --- |
| Cluster | k3d-kubevela, k3s v1.33.6 |
| KubeVela | `oamdev/vela-core:v1.11.0-alpha.3` |
| Fixture dir | `localtest/rttest/autoupdate-multi/` |
| Application | `multi-app` in namespace `default`, annotated `app.oam.dev/autoUpdate: "true"` |
| References | `mk-cd` (Component), `mk-trait` (Trait), `mk-policy` (Policy), `mk-step` (WorkflowStep) |

Each definition has a v1 and a v2 file. Only the marker field changes
between versions, so when the App re-renders we can read the marker
straight off the rendered ConfigMap (for CD and Trait), off the
policy-rendered ConfigMap (for Policy), or off the AppRev list (for
WorkflowStep).

## Answers to your questions

### Summary matrix

Each row is the live observation after applying the v2 file for that
kind while the App carries `autoUpdate: "true"`. The control row is
the same operation with the annotation removed.

| Test | Definition mutated | autoUpdate? | New AppRev cut? | Visible marker change |
| --- | --- | --- | --- | --- |
| 1 | ComponentDefinition `mk-cd` v1→v2 | Yes | Yes (`multi-app-v2`) | `multi-cm.data.cdMarker` v1 → v2 |
| 2 | TraitDefinition `mk-trait` v1→v2 | Yes | Yes (`multi-app-v3`) | `multi-cm` annotation `mk-trait/marker` v1 → v2 |
| 3 | PolicyDefinition `mk-policy` v1→v2 | Yes | Yes (`multi-app-v4`) | `mk-policy-marker.data.policyMarker` v1 → v2 |
| 4 | WorkflowStepDefinition `mk-step` v1→v2 | Yes | Yes (`multi-app-v5`) | No rendered marker for this kind; AppRev hash proves the change was noticed |
| Control | ComponentDefinition `mk-cd` v2→v1 | No (annotation removed) | No | Rendered manifest still `cdMarker: "v2"` — live CD already at v1, but the cached RT manifest is what the apply path reads |

### Q1. Does autoUpdate cover ComponentDefinitions?

Yes. Mutating the CD from v1 to v2 cut `multi-app-v2`. The rendered
ConfigMap's `cdMarker` field moved from `"v1"` to `"v2"` on the next
reconcile.

### Q2. Does autoUpdate cover TraitDefinitions?

Yes. Mutating the trait from v1 to v2 cut `multi-app-v3`. The ConfigMap
that the CD renders picked up the new annotation `mk-trait/marker: v2`
on the same reconcile.

### Q3. Does autoUpdate cover PolicyDefinitions?

Yes. Mutating `mk-policy` cut `multi-app-v4`. The custom policy in our
fixture renders its own ConfigMap (`mk-policy-marker`), and its
`policyMarker` field moved from `"v1"` to `"v2"`. Built-in policies
(garbage-collect, topology, apply-once, etc.) go through a different
parse path and do not produce their own rendered output, so for them
the AppRev hash is the only visible signal — but the gate is the same.

### Q4. Does autoUpdate cover WorkflowStepDefinitions?

Yes. Mutating `mk-step` cut `multi-app-v5`. The step itself is a
no-op message builder in our fixture, so nothing visible changes in
the cluster, but the AppRev hash bump and the new revision name are
proof that the controller noticed the change.

### Q5. What about WorkloadDefinitions?

Covered as well. We did not run a separate test for it because
WorkloadDefinition is an older, less-used kind in 1.10+, but the same
gate in the controller hashes its spec.

### Q6. What about without the annotation?

Without `autoUpdate: "true"` the controller only re-cuts an AppRev on
changes to the App's own spec (or on a `publishVersion` bump). The
control test reverted the CD from v2 back to v1 with the annotation
removed; the live CD updated but no new AppRev appeared, and the
rendered ConfigMap kept showing `cdMarker: "v2"` because the apply
pipeline reads from the ResourceTracker zstd cache, which still holds
the v2-era render.

## Why this is the behaviour (code path)

Two places in
`pkg/controller/core.oam.dev/v1beta1/application/revision.go` decide
whether a new AppRev is cut.

`currentAppRevIsNew` (around line 369):

```go
isLatestRev := deepEqualAppInRevision(h.latestAppRev, h.currentAppRev)
if metav1.HasAnnotation(h.app.ObjectMeta, oam.AnnotationAutoUpdate) {
    isLatestRev = h.app.Status.LatestRevision.RevisionHash == h.currentRevHash &&
                  DeepEqualRevision(h.latestAppRev, h.currentAppRev)
}
```

Default path (no autoUpdate): only `deepEqualAppInRevision` runs.
That function compares `Spec.Policies` (the App's *policy instances*),
`Spec.Workflow` (the App's *workflow instance*), and the App's own
`Spec`. It does not look at any definition body. So definition changes
go unnoticed.

autoUpdate path: the controller also requires
`h.app.Status.LatestRevision.RevisionHash == h.currentRevHash` to hold.
That hash is built by `ComputeAppRevisionHash` (line 275). The hash
struct includes a separate hash per kind:

```go
revHash := struct {
    ApplicationSpecHash        string
    WorkloadDefinitionHash     map[string]string
    ComponentDefinitionHash    map[string]string
    TraitDefinitionHash        map[string]string
    ScopeDefinitionHash        map[string]string
    PolicyDefinitionHash       map[string]string
    WorkflowStepDefinitionHash map[string]string
    PolicyHash                 map[string]string
    WorkflowHash               string
    ReferredObjectsHash        string
}{...}
```

If any definition's `Spec` changes, its bucket hash changes, the outer
hash changes, the equality check fails, and the controller cuts a new
AppRev. That is the mechanism. `DeepEqualRevision`, the secondary
check on the autoUpdate path, only compares Workload, Component, and
Trait definitions explicitly — but by the time the controller reaches
it, the hash check has already done the broader job.

## Reproducing this on your own cluster

```bash
# Apply baseline (v1 of everything)
kubectl apply -f localtest/rttest/autoupdate-multi/10-cd-v1.yaml
kubectl apply -f localtest/rttest/autoupdate-multi/11-trait-v1.yaml
kubectl apply -f localtest/rttest/autoupdate-multi/12-policy-v1.yaml
kubectl apply -f localtest/rttest/autoupdate-multi/13-wfstep-v1.yaml
kubectl apply -f localtest/rttest/autoupdate-multi/20-app.yaml

# Wait until you see `multi-app-v1` SUCCEEDED=true
kubectl get apprev -n default
kubectl get cm multi-cm mk-policy-marker -n default -o jsonpath='{.items[*].data}{"\n"}'

# Now mutate one definition at a time and re-check:
kubectl apply -f localtest/rttest/autoupdate-multi/30-cd-v2.yaml      # cuts v2
kubectl apply -f localtest/rttest/autoupdate-multi/31-trait-v2.yaml   # cuts v3
kubectl apply -f localtest/rttest/autoupdate-multi/32-policy-v2.yaml  # cuts v4
kubectl apply -f localtest/rttest/autoupdate-multi/33-wfstep-v2.yaml  # cuts v5

# Control: remove the annotation, revert CD — should produce no new AppRev:
kubectl annotate app multi-app -n default app.oam.dev/autoUpdate-
kubectl apply -f localtest/rttest/autoupdate-multi/10-cd-v1.yaml
# kubectl get apprev still shows multi-app-v5 as latest.
```

## Practical implications

- One annotation suffices for an Application that uses any mix of
  CD / Trait / Policy / WorkflowStep / Workload definitions. You do
  not need separate switches per definition kind.
- The cost is real: any definition you reference, when mutated by an
  operator change or a Helm upgrade of the platform addons, will cause
  your App to re-render automatically. For most workloads this is the
  point. For workloads that need pinned definition versions, leave the
  annotation off and bump `publishVersion` when you explicitly want
  to roll forward.
- The gate is the hash, not deep-equality. That makes it cheap to
  evaluate (one hash compare) and catches every kind. A finding that
  looks like "AppRev cut for no reason" is almost always a definition
  somewhere in the App's reference set that got mutated upstream.
