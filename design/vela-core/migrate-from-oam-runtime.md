# Migrate from OAM Kubernetes Runtime

Now we are refactoring OAM runtime in vela-core with CUE based abstractions. All source code from [oam-kubernetes-runtime](https://github.com/crossplane/oam-kubernetes-runtime)
has already been merged into [vela-core](https://github.com/oam-dev/kubevela/pull/663) now. Here is the doc for users who want
to migrate from OAM Runtime to use KubeVela.

## For Users Who are Using OAM runtime as Standalone Controller

If you are using OAM Runtime as Standalone Controller, migrating to KubeVela is very straight forward.

Server-side KubeVela(We call it `vela-core` for convenience) now includes following **BUILT-IN** CRDs and controllers.

| Type |  CRD   | Controller  | From |
| ---- |  ----  | ----  | ----  |
| Framework | `applicationconfigurations.core.oam.dev` | Yes | OAM Runtime |
| Framework | `components.core.oam.dev` | No | OAM Runtime |
| Workload | `containerizedworklaods.core.oam.dev` | Yes | OAM Runtime |
| Scope | `healthscope.core.oam.dev` | Yes | OAM Runtime |
| Trait | `manualscalertraits.core.oam.dev` | Yes | OAM Runtime |
| Framework | `scopedefinitions.core.oam.dev` | No | OAM Runtime |
| Framework | `traitdefinitions.core.oam.dev` | No | OAM Runtime |
| Framework | `workloaddefinitions.core.oam.dev` | No | OAM Runtime |
| Trait | `autoscalers.standard.oam.dev` | Yes | New in KubeVela |
| Trait | `metricstraits.standard.oam.dev` | Yes | New in KubeVela |
| Workload | `podspecworkloads.standard.oam.dev` | Yes | New in KubeVela |
| Trait | `route.standard.oam.dev` | Yes | New in KubeVela |

CRDs and Controllers in the table from 'OAM Runtime' are exactly the same to those in `oam-kubernetes-runtime`.
So in KubeVela we have added 4 more new CRDs with controller. 

### Option 1: You want only Pure OAM Runtime by using vela-core with No additional traits and workloads.

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
- add an args `- "--disable-caps=all"`, which will disable all additional workloads and traits built in vela-core.

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

### Option 2: You want use fully functioned KubeVela

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

5. Install Vela Dependency ConfigMap

```shell script
$ kubectl apply -f charts/vela-core/templates/velaConfig.yaml
```

6. Delete your old oam-runtime deployment

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

7. Install Certificate and Webhook for the new controller

```shell script
$ helm template --release-name kubevela -n vela-system -s templates/webhook.yaml charts/vela-core/ | kubectl apply -f -
```

8. Install the new controller

```shell script
$ helm template --release-name kubevela -n vela-system -s templates/kubevela-controller.yaml charts/vela-core/ | kubectl apply -f -
```

Then you have successfully migrate from oam-kubernetes-runtime to KubeVela.

## For Users Who are Using OAM runtime as Code Library

If you are using `oam-kubernetes-runtime` as code library, you can update your import headers.

Files are refactored as below:

| OLD |  NEW   | Usage  |
| ---- |  ----  | ----  |
| `github.com/crossplane/oam-kubernetes-runtime/apis/core` | `github.com/oam-dev/kubevela/apis/core.oam.dev` | API Spec Code |
| `github.com/crossplane/oam-kubernetes-runtime/pkg/controller` | `github.com/oam-dev/kubevela/pkg/controller/core.oam.dev` | OAM Controller Code |
| `github.com/crossplane/oam-kubernetes-runtime/pkg/oam` | `github.com/oam-dev/kubevela/pkg/oam` | OAM Common Lib Code |
| `github.com/crossplane/oam-kubernetes-runtime/pkg/webhook` | `github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev` | OAM Webhook Code |
