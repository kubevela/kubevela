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
the Deployment with bytes that were computed back on 1.10.6.

### Q2. What if I change the App spec?

The validating webhook rejects the request before etcd is touched. It
runs the same CUE eval up front as part of admission, hits the same
list-concat error, and returns HTTP 400 with the error verbatim in the
response body. The user sees the error immediately, the App and the
Deployment stay exactly as they were.

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
detection, rollback, and audit. It is not the runtime cache.

### Q5. What does the health status show?

`services[0].healthy: true`, the whole time. Same value as before the
upgrade. The health-collection path is the only place CUE compilation
fires on a steady-state reconcile, and when it fails it returns an
error to the caller. The caller logs the error and moves on without
updating `services[*].healthy`, so the field keeps the value it had on
the last successful collection — which in our case was `true` from the
1.10.6 era. If the upgrade had happened while the app was unhealthy, it
would stay unhealthy with the same indifference.

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
