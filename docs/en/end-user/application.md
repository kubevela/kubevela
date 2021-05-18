---
title: Application
---

This documentation will walk through how to use KubeVela to design a simple application without any polices or placement rule defined.

> Note: since you didn't declare placement rule, KubeVela will deploy this application directly to the control plane cluster (i.e. the cluster your `kubectl` is talking to). This is also the same case if you are using local cluster such as KinD or MiniKube to play KubeVela.

## Step 1: Check Available Components

Components are deployable or provisionable entities that compose your application. It could be a Helm chart, a simple Kubernetes workload, a CUE or Terraform module, or a cloud database etc.

Let's check the available components in fresh new KubeVela.

```shell
kubectl get comp -n vela-system
NAME              WORKLOAD-KIND   DESCRIPTION                        
task              Job             Describes jobs that run code or a script to completion.                                                                                          
webservice        Deployment      Describes long-running, scalable, containerized services that have a stable network endpoint to receive external network traffic from customers. 
worker            Deployment      Describes long-running, scalable, containerized services that running at backend. They do NOT have network endpoint to receive external network traffic.
```

To show the specification for given component, you could use `vela show`. 

```shell
$ kubectl vela show webservice
# Properties
+------------------+----------------------------------------------------------------------------------+-----------------------+----------+---------+
|       NAME       |                                   DESCRIPTION                                    |         TYPE          | REQUIRED | DEFAULT |
+------------------+----------------------------------------------------------------------------------+-----------------------+----------+---------+
| cmd              | Commands to run in the container                                                 | []string              | false    |         |
| env              | Define arguments by using environment variables                                  | [[]env](#env)         | false    |         |
| addRevisionLabel |                                                                                  | bool                  | true     | false   |
| image            | Which image would you like to use for your service                               | string                | true     |         |
| port             | Which port do you want customer traffic sent to                                  | int                   | true     |      80 |
| cpu              | Number of CPU units for the service, like `0.5` (0.5 CPU core), `1` (1 CPU core) | string                | false    |         |
| volumes          | Declare volumes and volumeMounts                                                 | [[]volumes](#volumes) | false    |         |
+------------------+----------------------------------------------------------------------------------+-----------------------+----------+---------+
... // skip other fields
```

> Tips: `vela show xxx --web` will open its capability reference documentation in your default browser.

You could always [add more components](components/more) to the platform at any time.

## Step 2: Declare an Application

Application is the full description of a deployment. Let's define an application that deploys a *Web Service* and a *Worker* components.

```yaml
# sample.yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: website
spec:
  components:
    - name: frontend
      type: webservice
      properties:
        image: nginx
    - name: backend
      type: worker
      properties:
        image: busybox
        cmd:
          - sleep
          - '1000'
```

## Step 3: Attach Traits

Traits are platform provided features that could *overlay* a given component with extra operational behaviors.

```shell
$ kubectl get trait -n vela-system
NAME                                       APPLIES-TO            DESCRIPTION                                     
cpuscaler                                  [webservice worker]   Automatically scale the component based on CPU usage.
ingress                                    [webservice worker]   Enable public web traffic for the component.
scaler                                     [webservice worker]   Manually scale the component.
sidecar                                    [webservice worker]   Inject a sidecar container to the component.
```

Let's check the specification of `sidecar` trait.

```shell
$ kubectl vela show sidecar
# Properties
+---------+-----------------------------------------+----------+----------+---------+
|  NAME   |               DESCRIPTION               |   TYPE   | REQUIRED | DEFAULT |
+---------+-----------------------------------------+----------+----------+---------+
| name    | Specify the name of sidecar container   | string   | true     |         |
| image   | Specify the image of sidecar container  | string   | true     |         |
| command | Specify the commands run in the sidecar | []string | false    |         |
+---------+-----------------------------------------+----------+----------+---------+
```

Note that traits are designed to be *overlays*.

This means for `sidecar` trait, your `frontend` component doesn't need to have a sidecar template or bring a webhook to enable sidecar injection. Instead, KubeVela is able to patch a sidecar to its workload instance after it is generated by the component (no matter it's a Helm chart or CUE module) but before it is applied to runtime cluster.

Similarly, the system will assign a HPA instance based on the properties you set and "link" it to the target workload instance, the component itself is untouched.

Now let's attach `sidecar` and `cpuscaler` traits to the `frontend` component. 

```yaml
# sample.yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: website
spec:
  components:
    - name: frontend              # This is the component I want to deploy
      type: webservice
      properties:
        image: nginx
      traits:
        - type: cpuscaler         # Automatically scale the component by CPU usage after deployed
          properties:
            min: 1
            max: 10
            cpuPercent: 60
        - type: sidecar           # Inject a fluentd sidecar before applying the component to runtime cluster
          properties:
            name: "sidecar-test"
            image: "fluentd"
    - name: backend
      type: worker
      properties:
        image: busybox
        cmd:
          - sleep
          - '1000'
```

## Step 4: Deploy the Application

```shell
$ kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/docs/examples/enduser/sample.yaml
application.core.oam.dev/website created
```

You'll get the application becomes `running`.

```shell
$ kubectl get application
NAME        COMPONENT   TYPE         PHASE     HEALTHY   STATUS   AGE
website     frontend    webservice   running   true               4m54s
```

Check the details of the application.

```shell
$ kubectl get app website -o yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  generation: 1
  name: website
  namespace: default
spec:
  components:
  - name: frontend
    properties:
      image: nginx
    traits:
    - properties:
        cpuPercent: 60
        max: 10
        min: 1
      type: cpuscaler
    - properties:
        image: fluentd
        name: sidecar-test
      type: sidecar
    type: webservice
  - name: backend
    properties:
      cmd:
      - sleep
      - "1000"
      image: busybox
    type: worker
status:
  ...
  latestRevision:
    name: website-v1
    revision: 1
    revisionHash: e9e062e2cddfe5fb
  services:
  - healthy: true
    name: frontend
    traits:
    - healthy: true
      type: cpuscaler
    - healthy: true
      type: sidecar
  - healthy: true
    name: backend
  status: running
```

Specifically:

1. `status.latestRevision` declares current revision of this deployment.
2. `status.services` declares the component created by this deployment and the healthy state.
3. `status.status` declares the global state of this deployment. 

### List Revisions

When updating an application entity, KubeVela will create a new revision for this change.

```shell
$ kubectl get apprev -l app.oam.dev/name=website
NAME           AGE
website-v1     35m
```

Furthermore, the system will decide how to/whether to rollout the application based on the attached [rollout plan](scopes/rollout-plan).
