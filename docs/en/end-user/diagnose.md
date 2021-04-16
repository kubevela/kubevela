---
title: Debug and Test
---

You can make further debug and test for your application by using [vela kubectl plugin](./kubectlplugin).

## Dry-Run the `Application`

Dry run will help you to understand what are the real resources which will to be expanded and deployed
to the Kubernetes cluster. In other words, it will mock to run the same logic as KubeVela's controller 
and output the results locally.

For example, let's dry-run the following application:

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

In this example, the definitions(`webservice` and `ingress`) which `vela-app` depends on is the built-in 
components and traits of KubeVela. You can also use `-d `or `--definitions` to specify your local definition files.

`-d `or `--definitions` permitting user to provide capability definitions used in the application from local files.
`dry-run` cmd will prioritize the provided capabilities than the living ones in the cluster.

## Live-Diff the `Application`

Live-diff helps you to have a preview of what would change if you're going to upgrade an application without making any changes
to the living cluster.
This feature is extremely useful for serious production deployment, and make the upgrade under control

It basically generates a diff between the specific revision of running instance and the local candidate application.
The result shows the changes (added/modified/removed/no_change) of the application as well as its sub-resources, 
such as components and traits.

Assume you have just deployed the application in dry-run section.
Then you can list the revisions of the Application.

```shell
$ kubectl get apprev -l app.oam.dev/name=vela-app
NAME          AGE
vela-app-v1   50s
```

Assume we're going to upgrade the application like below.

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

Run live-diff like this:

```shell
kubectl vela live-diff -f new-app.yaml -r vela-app-v1
```

`-r` or `--revision` is a flag that specifies the name of a living ApplicationRevision with which you want to compare the updated application.

`-c` or `--context` is a flag that specifies the number of lines shown around a change. The unchanged lines 
which are out of the context of a change will be omitted. It's useful if the diff result contains a lot of unchanged content 
while you just want to focus on the changed ones.

<details><summary> Click to view the details of diff result </summary>

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