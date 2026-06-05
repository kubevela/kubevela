# KubeVela 1.10.6 → 1.11 CUE upgrade — what actually happens to a running app

## TL;DR

A KubeVela Application built on a ComponentDefinition whose CUE
template compiled under the previous KubeVela's CUE engine but no
longer compiles under the new one keeps running through the upgrade.
The CUE error fires inside the health-collection sub-pipeline on every
reconcile, gets logged at error level, and is silently swallowed. The
only thing that breaks loudly is any attempt to edit the App spec —
the validating webhook rejects the request because the CD template no
longer compiles under the new CUE engine. The behaviour is the same
for any kind of CUE breaking change between two KubeVela versions and
applies to any workload kind the CD renders, not specific to a
particular operator, expression, or resource type.

## Setup

| Item | Value |
| --- | --- |
| Cluster | k3d-kubevela, k3s v1.33.6 |
| KubeVela 1.10.6 | `oamdev/vela-core:v1.10.6` (CUE 0.9 era) |
| KubeVela 1.11.0-alpha.3 | `oamdev/vela-core:v1.11.0-alpha.3` (CUE 0.14) |
| Install commands | `vela install --version 1.10.6 --yes` then `vela install --version 1.11.0-alpha.3 --yes` |

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

## Answers to your questions

### Summary matrix

One row per question. Each row describes how the KubeVela controller
behaves when an existing Application's ComponentDefinition stops
compiling after a KubeVela upgrade. The same pattern applies regardless
of which CUE breaking change triggered it or which workload kind the CD
renders. Detail and evidence for each row are in the Q1–Q5 sections
that follow.

| # | Scenario | CUE re-evaluated? | App status changes? | Underlying workload changes? | User-visible signal |
| --- | --- | --- | --- | --- | --- |
| Q1 | Reconcile post-upgrade, no spec touched | Yes — only in the health-collection sub-pipeline; error is logged and swallowed | No | No. Apply path patches from the ResourceTracker zstd cache | None on the user surface. Only the controller log shows the CUE error |
| Q2 | `kubectl patch` / `kubectl edit` the App | Yes — synchronously at the validating webhook | No (request rejected, nothing persists) | No | HTTP 400 with the full CUE error returned to the caller |
| Q3 | Where CUE is read on each reconcile path | Parse: live CD text loaded, not compiled. Apply: RT zstd cache, no CUE. Health: live CD re-evaluated against current parameters | N/A | N/A | N/A. The AppRev's `componentDefinitions[...]` snapshot is present but not read by any reconcile path |
| Q4 | `kubectl get app` health field | No. Last-good value is carried forward when the health-collection eval fails | No | No | `services[*].healthy` keeps reading `true` even though a real CUE failure fires every 30s |
| Q5 | Delete a rendered resource (Crossplane Claim, Deployment, etc.) | Yes — health-only, swallowed. Apply pipeline calls `Create()` with cached bytes | No | Yes — resource is recreated with a new UID and identical spec | None. App stays `running` / `healthy: true` through the gap |

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

### Q3. During reconciliation and health check, where does CUE get read from?

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

### Q4. What does the health status show?

`services[0].healthy: true`, the whole time. Same value as before the
upgrade. The health-collection path is the only place CUE compilation
fires on a steady-state reconcile, and when it fails it returns an
error to the caller. The caller logs the error and moves on without
updating `services[*].healthy`, so the field keeps the value it had on
the last successful collection — which in our case was `true` from the
1.10.6 era. If the upgrade had happened while the app was unhealthy, it
would stay unhealthy with the same indifference.

### Q5. What if I delete a rendered resource (e.g., a Crossplane S3 Claim)?

The KubeVela controller re-creates it from the ResourceTracker zstd
cache on the next reconcile. CUE is not invoked. We proved this live by
deleting the `victim-pod` Deployment after the upgrade: within 40
seconds it was back, with a new UID, identical spec, and `App` status
unchanged. The controller log shows `apply.go:126 "creating object"`
right after the health-collection CUE error fired and was swallowed —
the apply pipeline went ahead with cached bytes regardless.

What this means in different deletion scenarios:

| Action | What KubeVela does | CUE involved? | Outcome |
| --- | --- | --- | --- |
| `kubectl delete bucket my-s3` (delete the Claim that KubeVela rendered) | Sees the Claim missing, reads RT zstd cache, calls `Create()` with the cached Claim manifest | No | Claim comes back with a new UID. Crossplane sees the new Claim and (re)creates the underlying S3 bucket. |
| Someone deletes the actual S3 bucket in AWS (Claim CR still in the cluster) | KubeVela tracks only the Claim, not the bucket. Reconcile patches the Claim with cached bytes — a no-op since the Claim is unchanged | No | Crossplane is responsible for noticing the drift and reconciling the bucket. KubeVela has nothing to say. |
| `kubectl delete app victim` | Controller deletes the Claim through ResourceTracker GC | No | Claim deleted, Crossplane garbage-collects the bucket. |

The cached manifest carries forward the original annotations
(`oam.dev/kubevela-version: v1.10.6`, `app.oam.dev/app-revision-hash`,
etc.). A recreated Claim looks identical to the deleted one except for
UID and resourceVersion. Crossplane's composition logic treats it as
the same Claim.

The silent-failure edge here is when the Claim exists in etcd but the
underlying S3 bucket was never (or no longer) provisioned — for
example if the Crossplane provider was scaled to zero, or its cloud
credentials were rotated and lost. KubeVela's RT still shows the Claim
as applied, `services[0].healthy` keeps reading `true`, and there is
no signal that the actual bucket is absent. CUE has nothing to do with
this — it is a layer-mismatch problem that exists at every version.

