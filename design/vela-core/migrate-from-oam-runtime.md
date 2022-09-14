# How to upgrade to KubeVela

What if I want to upgrade from [oam-kubernetes-runtime](https://github.com/crossplane/oam-kubernetes-runtime) to KubeVela? Here's a detailed guide!

## For users who are using OAM runtime as standalone controller

If you are using OAM Runtime as Standalone Controller, upgrading to KubeVela to KubeVela is very straight forward.

Server-side KubeVela(We call it `vela-core` for convenience) now includes following **BUILT-IN** CRDs and controllers.

| Type |  CRD   | Controller  | From |
| ---- |  ----  | ----  | ----  |
| Control Plane Object | `applicationconfigurations.core.oam.dev` | Yes | OAM Runtime |
| Control Plane Object | `components.core.oam.dev` | Yes | OAM Runtime |
| Workload Type | `containerizedworklaods.core.oam.dev` | Yes | OAM Runtime |
| Scope | `healthscope.core.oam.dev` | Yes | OAM Runtime |
| Control Plane Object | `scopedefinitions.core.oam.dev` | No | OAM Runtime |
| Control Plane Object | `traitdefinitions.core.oam.dev` | No | OAM Runtime |
| Control Plane Object | `workloaddefinitions.core.oam.dev` | No | OAM Runtime |
| Trait | `autoscalers.standard.oam.dev` | Yes | New in KubeVela |
| Trait | `metricstraits.standard.oam.dev` | Yes | New in KubeVela |
| Workload Type | `podspecworkloads.standard.oam.dev` | Yes | New in KubeVela |
| Trait | `route.standard.oam.dev` | Yes | New in KubeVela |

CRDs and Controllers in the table from 'OAM Runtime' are exactly the same to those in `oam-kubernetes-runtime`.
So in KubeVela we have added 4 more new CRDs with controller. 

### Option 1: I only want to have OAM control plane objects only, no additional traits and workload types.

1. Find you deployment

```shell script
$ kubectl -n oam-system get deployment -l app.kubernetes.io/name=oam-kubernetes-runtime
NAME                         READY   UP-TO-DATE   AVAILABLE   AGE
oam-kubernetes-runtime-oam   1/1     1            1           62s
```

2. Update the deployment

In this case, the deployment name of OAM runtime is `oam-kubernetes-runtime-oam`, let's edit it to update the image:

```shell script
$ kubectl -n oam-system edit deployment oam-kubernetes-runtime-oam
```

There are two changes:

- update the image from `crossplane/oam-kubernetes-runtime:latest` to `oamdev/vela-core:latest`
- add an args `- "--disable-caps=all"`, which will disable all additional workloads and traits built in vela-core described in the following table.

| Type | Current KubeVela Additional CRD   |
| ---- |  ----  |
| Trait | `autoscalers.standard.oam.dev` |
| Trait | `metricstraits.standard.oam.dev` |
| Workload | `podspecworkloads.standard.oam.dev` |
| Trait | `route.standard.oam.dev` |

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: oam-kubernetes-runtime-oam
...
      containers:
        - name: oam
          args:
            - "--metrics-addr=:8080"
            - "--enable-leader-election"
+           - "--disable-caps=all"
-         image: crossplane/oam-kubernetes-runtime:latest
+         image: oamdev/vela-core:latest
          imagePullPolicy: "Always"
...
```

### Option 2: I want full featured KubeVela, including its built-in workload types and traits.

1. Install Additional CRDs

```shell script
$ kubectl apply -f charts/vela-core/crds
```

2. Install Definition files

```shell script
$ kubectl apply -f charts/vela-core/templates/defwithtemplate
```

3. Create namespace, vela-core use `vela-system` as default

```shell script
$ kubectl create ns vela-system
```

4. Install Cert Manager

```shell script
$ kubectl apply -f charts/vela-core/templates/cert-manager.yaml
```

5. Delete your old oam-runtime deployment

Find the running deployment.

```shell script
$ kubectl -n oam-system get deployment -l app.kubernetes.io/name=oam-kubernetes-runtime
NAME                         READY   UP-TO-DATE   AVAILABLE   AGE
oam-kubernetes-runtime-oam   1/1     1            1           62s
```

Delete the deployment found.

```shell script
$ kubectl -n oam-system delete deployment oam-kubernetes-runtime-oam
```

6. Install Certificate and Webhook for the new controller

```shell script
$ helm template --release-name kubevela -n vela-system -s templates/webhook.yaml charts/vela-core/ | kubectl apply -f -
```

7. Install the new controller

```shell script
$ helm template --release-name kubevela -n vela-system -s templates/kubevela-controller.yaml charts/vela-core/ | kubectl apply -f -
```

> TIPS: If you want to disable webhook, change 'useWebhook' to be 'false' in  `charts/vela-core/values.yaml`

Then you have successfully migrate from oam-kubernetes-runtime to KubeVela.

## For users who are importing OAM runtime as library

If you are importing `oam-kubernetes-runtime` as library, you can update your import headers.

Files are refactored as below:

| OLD |  NEW   | Usage  |
| ---- |  ----  | ----  |
| `github.com/crossplane/oam-kubernetes-runtime/apis/core` | `github.com/oam-dev/kubevela/apis/core.oam.dev` | API Spec Code |
| `github.com/crossplane/oam-kubernetes-runtime/pkg/controller` | `github.com/oam-dev/kubevela/pkg/controller/core.oam.dev` | OAM Controller Code |
| `github.com/crossplane/oam-kubernetes-runtime/pkg/oam` | `github.com/oam-dev/kubevela/pkg/oam` | OAM Common Lib Code |
| `github.com/crossplane/oam-kubernetes-runtime/pkg/webhook` | `github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev` | OAM Webhook Code |
