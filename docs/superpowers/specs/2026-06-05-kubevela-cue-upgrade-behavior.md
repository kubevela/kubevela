# KubeVela 1.10.6 → 1.11 CUE upgrade — what actually happens to a running app

## TL;DR

A KubeVela 1.10.6 Application built on a ComponentDefinition that uses
CUE list-concat (`a + b`) keeps running after you upgrade to 1.11.0-alpha.3
on CUE 0.14. The pod does not restart, the Deployment UID does not
change, and `kubectl get app` still says `healthy: true`. The CUE error
fires inside the health-collection sub-pipeline on every reconcile, gets
logged at error level, and is silently swallowed. The only thing that
breaks loudly is any attempt to edit the App spec — the validating
webhook rejects the request because the CD template no longer compiles
under CUE 0.14.

## Setup

| Item | Value |
| --- | --- |
| Cluster | k3d-kubevela, k3s v1.33.6 |
| KubeVela 1.10.6 | `oamdev/vela-core:v1.10.6` (CUE 0.9 era) |
| KubeVela 1.11.0-alpha.3 | `oamdev/vela-core:v1.11.0-alpha.3` (CUE 0.14) |
| Resync | `--application-re-sync-period=30s` (tighter than the 5-min default) |
| Install commands | `vela install --version 1.10.6 --yes` then `vela install --set controllerArgs.reSyncPeriod=30s --version 1.11.0-alpha.3 --yes` |

### Fixture: bad-cd

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: bad-cd
  namespace: vela-system
spec:
  workload:
    definition: { apiVersion: apps/v1, kind: Deployment }
  schematic:
    cue:
      template: |
        output: {
          apiVersion: "apps/v1"
          kind:       "Deployment"
          metadata: name: context.name
          spec: {
            replicas: parameter.replicas
            selector: matchLabels: "app.oam.dev/component": context.name
            template: {
              metadata: labels: "app.oam.dev/component": context.name
              spec: containers: [{
                name:    "main"
                image:   parameter.image
                command: parameter.preArgs + parameter.postArgs   # <-- the bomb
              }]
            }
          }
        }
        parameter: {
          image:    string
          replicas: *1 | int
          preArgs:  [...string]
          postArgs: [...string]
        }
```

The `+` operator on two lists compiles fine on CUE 0.9. CUE 0.11 removed
it. CUE 0.14, used by KubeVela 1.11, rejects it with `Addition of lists
is superseded by list.Concat`.

### Fixture: victim Application

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: victim
  namespace: default
spec:
  components:
    - name: victim-pod
      type: bad-cd
      properties:
        image: nginx:1.27
        replicas: 1
        preArgs: ["sh", "-c"]
        postArgs: ["sleep 3600"]
```

On 1.10.6 this renders to a Deployment running `["sh","-c","sleep 3600"]`,
phase `running`, healthy `true`.

## Answers to your five questions

### Q1. Will reconciliation break the running app after the upgrade?

No. The Deployment, the pod, and the App status all survive. The
controller does hit a real CUE 0.14 error on every reconcile, but it
fires inside the health-collection sub-pipeline in
`applyComponentHealthToServices`
(`pkg/controller/core.oam.dev/v1beta1/application/application_controller.go`),
which logs the error and continues.
The main apply path never re-renders the CUE template; it reads a
pre-rendered manifest from the ResourceTracker's zstd cache and patches
the Deployment with bytes that were computed back on 1.10.6. See O1.

### Q2. What if I change the App spec?

The validating webhook rejects the request before etcd is touched. It
runs the same CUE eval up front as part of admission, hits the same
list-concat error, and returns HTTP 400 with the error verbatim in the
response body. The user sees the error immediately, the App and the
Deployment stay exactly as they were. See O2.

### Q3. What is the relationship between Application and ApplicationRevision?

The AppRev is an immutable snapshot of everything the App needed at the
time it was last accepted: the App's spec, all referenced CDs, all
referenced policies, all referenced traits — each captured by value,
not by reference. A new AppRev is cut whenever the controller decides
the App has materially changed (spec edit, `publishVersion` bump, or
the `autoUpdate` annotation triggering a reconcile of dependencies).
The AppRev's name encodes the revision number (`victim-v1`,
`victim-v2`, etc.) and its hash is what the controller uses to decide
"is this still the same revision". When a spec edit is rejected by the
webhook, no AppRev is cut.

### Q4. During reconciliation and health check, where does CUE get read from?

Three different places, depending on the path:

| Path | Source of CUE/manifest | What happens on broken CUE |
| --- | --- | --- |
| Parse (`parser.go`) | **Live ComponentDefinition** object | Read succeeds (it's just YAML); doesn't actually compile the CUE here |
| Apply (`apply.go:126`) | **ResourceTracker zstd cache** of already-rendered manifests | No CUE involved; patches the live Deployment with cached bytes |
| Health collection (`applyComponentHealthToServices`) | **Live CD again**, re-evaluates CUE template against current parameters | Fails with the CUE 0.14 error; error is logged and swallowed |
| AppRev `componentDefinitions[bad-cd]` snapshot | exists, but **not read** by any of the above paths during steady-state reconcile |  |

The AppRev's snapshot of the CD template is real (we verified the exact
broken line is in the AppRev YAML), but the reconciler does not read
from it. Delete the live CD and the parse path fails immediately with
`WorkloadDefinition.core.oam.dev "bad-cd" not found`, even though the
AppRev still has the template byte-for-byte. The AppRev is for change
detection, rollback, and audit. It is not the runtime cache. See O3.

### Q5. What does the health status show?

`services[0].healthy: true`, the whole time. Same value as before the
upgrade. The health-collection path is the only place CUE compilation
fires on a steady-state reconcile, and when it fails it returns an
error to the caller. The caller logs the error and moves on without
updating `services[*].healthy`, so the field keeps the value it had on
the last successful collection — which in our case was `true` from the
1.10.6 era. If the upgrade had happened while the app was unhealthy, it
would stay unhealthy with the same indifference. See O4.

## Observations

### O1 — untouched reconcile after upgrade

Five reconciles fired over 90 seconds (30s resync):

```
04:47:49.133643  Start reconcile application victim
04:48:19.552629  Start reconcile application victim
04:48:49.572178  Start reconcile application victim
04:49:19.590102  Start reconcile application victim
04:49:49.607802  Start reconcile application victim
```

Every reconcile emitted the same error (line number is from the
running 1.11.0-alpha.3 binary; current main is at the
`applyComponentHealthToServices` function in the same file):

```
E0605 04:48:19.564769  application_controller.go:905
"Failed to collect health status" err=<
  GenerateComponentManifest: evaluate base template app=victim in namespace=default:
  validation failed for workload victim-pod:
  Template errors:
    output.spec.template.spec.containers.0.command:
      Addition of lists is superseded by list.Concat;
      see https://cuelang.org/e/v0.11-list-arithmetic
>
```

Right after the error, the same reconcile patched the Deployment
anyway:

```
I0605 04:48:19.569515  apply.go:126  "patching object" name="victim-pod"
```

State delta vs. pre-upgrade baseline:

| Field | Pre-upgrade | Post-upgrade |
| --- | --- | --- |
| Deployment UID | `0110a06a-9248-491d-8bec-3f54009af970` | `0110a06a-9248-491d-8bec-3f54009af970` (identical) |
| Pod UID | `a240afa0-ccdc-4cfb-861b-7577b0e242ce` | `a240afa0-ccdc-4cfb-861b-7577b0e242ce` (identical) |
| Pod restartCount | 0 | 0 |
| App phase | running | running |
| `services[0].healthy` | true | true |
| All `status.conditions` | True | True (lastTransitionTime unchanged) |

Verdict: the application keeps running. The controller is shouting a
real CUE error into its log every 30 seconds and nothing on the user
surface reflects it.

### O2 — spec edit attempted after upgrade

Patch sent: flip `replicas` from 1 to 2 via `kubectl patch app victim
--type=merge`.

```
Error from server: admission webhook
"validating.core.oam.dev.v1beta1.applications" denied the request:
  1) "schematic": cannot create the validation process context
     of app=victim in namespace=default:
     evaluate base template app=victim in namespace=default:
     validation failed for workload victim-pod:
  Template errors:
    output.spec.template.spec.containers.0.command:
      Addition of lists is superseded by list.Concat
```

After the rejection:
- `.spec.components[0].properties.replicas` still 1
- AppRev list still has only `victim-v1` (no new revision cut)
- `.status.status` still `running`, `services[0].healthy` still `true`

Verdict: the webhook is the loud door. Any write attempt fails
immediately and visibly. The user cannot accidentally roll a change
forward while the CD is broken.

### O3 — where the CUE is read from

**Probe (a). Does the AppRev contain the CD's CUE template?**

```python
template = apprev['spec']['componentDefinitions']['bad-cd']['spec']['schematic']['cue']['template']
# 542 chars, contains 'command: parameter.preArgs + parameter.postArgs' verbatim
```

Yes. The full template, broken line and all, is in the AppRev.

**Probe (b). If we delete the live CD, does the App still reconcile
using the AppRev snapshot?**

```bash
kubectl delete componentdefinition bad-cd -n vela-system
kubectl rollout restart deploy/kubevela-vela-core -n vela-system   # fresh controller
```

Result on the next reconcile:

```
E0605 04:52:01  controller.go:316  "Reconciler error"
  err="failed to parseComponents: fetch component/policy type of victim-pod:
       load template from component definition [bad-cd]:
       WorkloadDefinition.core.oam.dev \"bad-cd\" not found"
```

No, it does not. The parse path looks the live CD up by name and
fails immediately when it's missing. The AppRev's snapshot of the same
template is sitting there unused.

The Deployment kept running through this window because the
ResourceTracker was already holding the rendered manifest and nothing
was tearing it down. After re-applying `bad-cd`, the App returned to
`phase: running` and the CUE health-collection error came back exactly
as in O1.

**Probe (c). Where is the rendered manifest then?**

In the ResourceTracker's zstd-compressed `spec.compression.data` field:

```python
import base64, zstandard
raw = base64.b64decode(rt['spec']['compression']['data'])   # 512 bytes
plain = zstandard.ZstdDecompressor().decompress(raw)         # 825 bytes
# plain is JSON: list of managed resources
# First entry:
#   kind=Deployment, name=victim-pod, namespace=default
#   raw.metadata.annotations: oam.dev/kubevela-version=v1.10.6
#   raw.spec.template.spec.containers[0].command=["sh","-c","sleep 3600"]
```

The `command` field is already the post-concat list. CUE did the
addition once back on 1.10.6, the result was stored, and the apply
pipeline has been patching the Deployment with this pre-rendered blob
ever since. CUE never runs on this path.

The annotation tag `oam.dev/kubevela-version: v1.10.6` is the receipt
showing when the render happened. It does not move on upgrade.

### O4 — health status across O1, O2, O3

| Step | `phase` | `services[0].healthy` | Conditions |
| --- | --- | --- | --- |
| Pre-upgrade baseline | running | true | all True |
| Five reconciles post-upgrade (O1) | running | true | all True, lastTransitionTime unchanged |
| Webhook-rejected patch (O2) | running | true | all True (no state change at all) |
| Live CD deleted (O3) | rendering | true | `Parsed: False` with "bad-cd not found"; other conditions still True from earlier |
| Live CD re-applied (O3) | running | true | back to all True |

The `healthy: true` carryover across every scenario is the headline.
The status surface is lying about a real problem, every 30 seconds, in
plain sight of `kubectl get app`.

## Why it works this way

Three layers, each doing its own thing:

1. **Validating webhook on Application admission.** It runs the full
   CUE eval up front. This is the loud, synchronous gate. Any write
   to an Application that fails CUE eval is rejected before the
   apiserver persists it. This is why O2 fails immediately and
   visibly.

2. **Reconciler's apply path.** It does not re-render CUE on every
   loop. The ResourceTracker holds a zstd-compressed copy of the
   rendered manifests from the last successful render. The apply path
   reads those bytes and three-way-merges them onto the live
   Deployment. This is why O1's Deployment never changes and never
   restarts. The CUE engine could be ripped out of the binary and this
   loop would still work.

3. **Reconciler's health-collection path.** This one does re-run the
   CUE eval to compute health. It calls `GenerateComponentManifest`
   against the live CD with the current parameters. When CUE fails,
   it returns an error to the caller in `application_controller.go`
   around line 905. The caller has this shape:

   ```go
   status, err := getComponentHealthStatus(...)
   if err != nil {
       log.Error(err, "Failed to collect health status")
   } else if status != nil {
       handler.services[idx].Healthy = status.Healthy
   }
   ```

   No retry, no propagation, no condition update. Just a log line. The
   `services[idx].Healthy` field is only touched on success, so on
   failure it carries forward whatever was there last.

The AppRev sits to the side of all three. It is the durable snapshot
that lets KubeVela tell whether the App has materially changed since
last time, lets users roll back to a known-good version, and lets
auditors see what was in flight at a given moment. The runtime
reconciler does not use it as the source of CUE templates — that
remains the live ComponentDefinition.

## What's still risky in production

- **Silent health surface.** The most dangerous part of this is not
  the upgrade itself, it is what comes after. A real CUE 0.14
  incompatibility is firing on every reconcile and no metric, no
  condition, and no `kubectl get app` column reflects it. Anyone
  watching the dashboards will see green. The only signal is the
  controller log, which is rarely the first place to look.
- **Drift detection is dead while CUE is broken.** The apply path
  patches the Deployment with the pre-upgrade manifest. If the
  rendered template would have changed for some other reason
  (security fix, new label, sidecar injection upstream of CUE), the
  cluster will continue running the stale shape. Reconciliation is
  not regenerating it.
- **The CD deletion behaviour is fragile.** During the broken-CUE
  window, deleting the live CD switches the App from "silently
  unhealthy" to "loudly stuck on parse". The Deployment keeps running
  by inertia. If the cluster is ever scaled down, restarted, or
  GC-pressured during this state, the App has no path to come back —
  there is no CD to load.
- **You can edit the App, but only the bits the webhook doesn't
  evaluate.** Status writes, label patches, owner-reference updates,
  GC actions — all of those bypass the validating webhook. Any code
  path that doesn't go through Application admission can mutate App
  state even while the CD is broken.

The recovery path is to rewrite the CD with `list.Concat` (or any
CUE 0.14-compatible expression). After re-admission, the next
reconcile re-renders, replaces the RT zstd payload with fresh bytes,
and the health-collection path stops erroring. We will verify this
explicitly in Approach 2.
