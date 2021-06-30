---
title: Dry-Run / Live-Diff
---

KubeVela 支持两种方式调试 application：dry-run 和 live-diff。


## Dry-Run `Application`

Dry-run 将帮助我们了解哪些资源将被处理并部署到 Kubernetes 集群。另外，该命令支持模拟运行与KubeVela的控制器相同的逻辑并在本地输出结果。

比如，我们 dry-run 下面 application：

```yaml
# app.yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: vela-app
spec:
  components:
    - name: express-server
      type: webservice
      properties:
        image: crccheck/hello-world
        port: 8000
      traits:
        - type: ingress
          properties:
            domain: testsvc.example.com
            http:
              "/": 8000
```

```shell
kubectl vela dry-run -f app.yaml
---
# Application(vela-app) -- Comopnent(express-server)
---

apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app.oam.dev/appRevision: ""
    app.oam.dev/component: express-server
    app.oam.dev/name: vela-app
    workload.oam.dev/type: webservice
spec:
  selector:
    matchLabels:
      app.oam.dev/component: express-server
  template:
    metadata:
      labels:
        app.oam.dev/component: express-server
    spec:
      containers:
      - image: crccheck/hello-world
        name: express-server
        ports:
        - containerPort: 8000

---
apiVersion: v1
kind: Service
metadata:
  labels:
    app.oam.dev/appRevision: ""
    app.oam.dev/component: express-server
    app.oam.dev/name: vela-app
    trait.oam.dev/resource: service
    trait.oam.dev/type: ingress
  name: express-server
spec:
  ports:
  - port: 8000
    targetPort: 8000
  selector:
    app.oam.dev/component: express-server

---
apiVersion: networking.k8s.io/v1beta1
kind: Ingress
metadata:
  labels:
    app.oam.dev/appRevision: ""
    app.oam.dev/component: express-server
    app.oam.dev/name: vela-app
    trait.oam.dev/resource: ingress
    trait.oam.dev/type: ingress
  name: express-server
spec:
  rules:
  - host: testsvc.example.com
    http:
      paths:
      - backend:
          serviceName: express-server
          servicePort: 8000
        path: /

---
```

当前示例中，application `vela-app` 依赖 KubeVela 内置的 component（`webservice`） 和 trait（`ingress`）。我们也可以通过参数 `-d ` 或者 `--definitions` 指定本地 definition 文件。

参数 `-d ` 或者 `--definitions` 允许用户从本地文件导入指定的 definitions 以供 application 使用。
参数 `dry-run` 会将优先使用用户指定的 capabilities 。

## Live-Diff `Application`

Live-diff 将帮助我们预览本次升级 application 会有哪些变更，同时不会对现有集群产生影响。
本功能对于生产环境变更非常有用，同时还能保证升级可控。

本功能会在线上正在运行的版本与本地待升级版本之间生成差异信息。
最终差异结果将展示 application 以及子资源（比如 components 以及 traits）的变更信息（added/modified/removed/no_change）。

假设我们在 dry-run 环节已经部署 application 。
随后，我们列出上面 application 的 revisions 信息。

```shell
$ kubectl get apprev -l app.oam.dev/name=vela-app
NAME          AGE
vela-app-v1   50s
```

假设我们将更新该 application ：

```yaml
# new-app.yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: vela-app
spec:
  components:
    - name: express-server
      type: webservice
      properties:
        image: crccheck/hello-world
        port: 8080 # change port
        cpu: 0.5 # add requests cpu units
    - name: my-task # add a component
      type: task
      properties:
        image: busybox
        cmd: ["sleep", "1000"]
      traits:
        - type: ingress
          properties:
            domain: testsvc.example.com
            http:
              "/": 8080 # change port
```

执行 live-diff ：

```shell
kubectl vela live-diff -f new-app.yaml -r vela-app-v1
```

参数 `-r` 或者 `--revision` 用于指定正在运行中的 ApplicationRevision 名称，该版本将用于与更新版本进行比较。

参数 `-c` or `--context` 用于指定显示变更上下文行数，超出上线行数的未变更行将被省略。该功能对于如下场景非常有用：差异结果包含很多未更改的内容，而我们只想关注已更改的内容。

<details><summary> diff </summary>

```bash
---
# Application (vela-app) has been modified(*)
---
  apiVersion: core.oam.dev/v1beta1
  kind: Application
  metadata:
    creationTimestamp: null
    name: vela-app
    namespace: default
  spec:
    components:
    - name: express-server
      properties:
+       cpu: 0.5
        image: crccheck/hello-world
-       port: 8000
+       port: 8080
+     type: webservice
+   - name: my-task
+     properties:
+       cmd:
+       - sleep
+       - "1000"
+       image: busybox
      traits:
      - properties:
          domain: testsvc.example.com
          http:
-           /: 8000
+           /: 8080
        type: ingress
-     type: webservice
+     type: task
  status:
    batchRollingState: ""
    currentBatch: 0
    rollingState: ""
    upgradedReadyReplicas: 0
    upgradedReplicas: 0

---
## Component (express-server) has been modified(*)
---
  apiVersion: core.oam.dev/v1alpha2
  kind: Component
  metadata:
    creationTimestamp: null
    labels:
      app.oam.dev/name: vela-app
    name: express-server
  spec:
    workload:
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        labels:
          app.oam.dev/appRevision: ""
          app.oam.dev/component: express-server
          app.oam.dev/name: vela-app
          workload.oam.dev/type: webservice
      spec:
        selector:
          matchLabels:
            app.oam.dev/component: express-server
        template:
          metadata:
            labels:
              app.oam.dev/component: express-server
          spec:
            containers:
            - image: crccheck/hello-world
              name: express-server
              ports:
-             - containerPort: 8000
+             - containerPort: 8080
  status:
    observedGeneration: 0

---
### Component (express-server) / Trait (ingress/service) has been removed(-)
---
- apiVersion: v1
- kind: Service
- metadata:
-   labels:
-     app.oam.dev/appRevision: ""
-     app.oam.dev/component: express-server
-     app.oam.dev/name: vela-app
-     trait.oam.dev/resource: service
-     trait.oam.dev/type: ingress
-   name: express-server
- spec:
-   ports:
-   - port: 8000
-     targetPort: 8000
-   selector:
-     app.oam.dev/component: express-server

---
### Component (express-server) / Trait (ingress/ingress) has been removed(-)
---
- apiVersion: networking.k8s.io/v1beta1
- kind: Ingress
- metadata:
-   labels:
-     app.oam.dev/appRevision: ""
-     app.oam.dev/component: express-server
-     app.oam.dev/name: vela-app
-     trait.oam.dev/resource: ingress
-     trait.oam.dev/type: ingress
-   name: express-server
- spec:
-   rules:
-   - host: testsvc.example.com
-     http:
-       paths:
-       - backend:
-           serviceName: express-server
-           servicePort: 8000
-         path: /

---
## Component (my-task) has been added(+)
---
+ apiVersion: core.oam.dev/v1alpha2
+ kind: Component
+ metadata:
+   creationTimestamp: null
+   labels:
+     app.oam.dev/name: vela-app
+   name: my-task
+ spec:
+   workload:
+     apiVersion: batch/v1
+     kind: Job
+     metadata:
+       labels:
+         app.oam.dev/appRevision: ""
+         app.oam.dev/component: my-task
+         app.oam.dev/name: vela-app
+         workload.oam.dev/type: task
+     spec:
+       completions: 1
+       parallelism: 1
+       template:
+         spec:
+           containers:
+           - command:
+             - sleep
+             - "1000"
+             image: busybox
+             name: my-task
+           restartPolicy: Never
+ status:
+   observedGeneration: 0

---
### Component (my-task) / Trait (ingress/service) has been added(+)
---
+ apiVersion: v1
+ kind: Service
+ metadata:
+   labels:
+     app.oam.dev/appRevision: ""
+     app.oam.dev/component: my-task
+     app.oam.dev/name: vela-app
+     trait.oam.dev/resource: service
+     trait.oam.dev/type: ingress
+   name: my-task
+ spec:
+   ports:
+   - port: 8080
+     targetPort: 8080
+   selector:
+     app.oam.dev/component: my-task

---
### Component (my-task) / Trait (ingress/ingress) has been added(+)
---
+ apiVersion: networking.k8s.io/v1beta1
+ kind: Ingress
+ metadata:
+   labels:
+     app.oam.dev/appRevision: ""
+     app.oam.dev/component: my-task
+     app.oam.dev/name: vela-app
+     trait.oam.dev/resource: ingress
+     trait.oam.dev/type: ingress
+   name: my-task
+ spec:
+   rules:
+   - host: testsvc.example.com
+     http:
+       paths:
+       - backend:
+           serviceName: my-task
+           servicePort: 8080
+         path: /
```

</details>