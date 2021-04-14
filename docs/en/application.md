---
title: Deploy Application
---

This documentation will walk through a full application deployment workflow on KubeVela platform.

## Introduction

KubeVela is a fully self-service platform. All capabilities an application deployment needs are maintained as building block modules in this platform. Specifically:
- Components - deployable/provisionable entities that composed your application deployment
  - e.g. a Kubernetes workload, a MySQL database, or a AWS OSS bucket
- Traits - attachable operational features per your needs
  - e.g. autoscaling rules, rollout strategies, ingress rules, sidecars, security policies etc

## Step 1: Check Capabilities in the Platform

As user of this platform, you could check available components you can deploy, and available traits you can attach.

```console
$ kubectl get componentdefinitions -n vela-system
NAME         WORKLOAD-KIND   DESCRIPTION                                                                                                                                                AGE
task         Job             Describes jobs that run code or a script to completion.                                                                                                    5h52m
webservice   Deployment      Describes long-running, scalable, containerized services that have a stable network endpoint to receive external network traffic from customers.           5h52m
worker       Deployment      Describes long-running, scalable, containerized services that running at backend. They do NOT have network endpoint to receive external network traffic.   5h52m
```

```console
$ kubectl get traitdefinitions -n vela-system
NAME      APPLIES-TO                DESCRIPTION                                                                                                                           AGE
ingress   ["webservice","worker"]   Configures K8s ingress and service to enable web traffic for your service. Please use route trait in cap center for advanced usage.   6h8m
cpuscaler ["webservice","worker"]   Configure k8s HPA with CPU metrics for Deployment                                                                                          6h8m
```

To show the specification for given capability, you could use `vela` CLI. For example, `vela show webservice` will return full schema of *Web Service* component and `vela show webservice --web` will open its capability reference documentation in your browser.

## Step 2: Design and Deploy Application

In KubeVela, `Application` is the main API to define your application deployment based on available capabilities. Every `Application` could contain multiple components, each of them can be attached with a number of traits per needs. 

Now let's define an application composed by *Web Service* and *Worker* components.

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
      traits:
        - type: cpuscaler
          properties:
            min: 1
            max: 10
            cpuPercent: 60
        - type: sidecar
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

In this sample, we also attached `sidecar` and `cpuscaler` traits to the `frontend` component.
So after deployed, the `frontend` component instance (a Kubernetes Deployment workload) will be automatically injected
with a `fluentd` sidecar and automatically scale from 1-10 replicas based on CPU usage.

### Deploy the Application

Apply application YAML to Kubernetes:

```shell
$ kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/docs/examples/enduser/sample.yaml
application.core.oam.dev/website created
```

You'll get the application becomes `running`.

```shell
$ kubectl get application website -o yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
 name: website
....
status:
  components:
  - apiVersion: core.oam.dev/v1alpha2
    kind: Component
    name: backend
  - apiVersion: core.oam.dev/v1alpha2
    kind: Component
    name: frontend
....
  status: running

```

### Verify the Deployment

You could see a Deployment named `frontend` is running, with port exposed, and with a container `fluentd` injected.

```shell
$ kubectl get deploy frontend
NAME       READY   UP-TO-DATE   AVAILABLE   AGE
frontend   1/1     1            1           97s
```

```shell
$ kubectl get deploy frontend -o yaml
...
    spec:
      containers:
      - image: nginx
        imagePullPolicy: Always
        name: frontend
        ports:
        - containerPort: 80
          protocol: TCP
      - image: fluentd
        imagePullPolicy: Always
        name: sidecar-test
...
```

Another Deployment is also running named `backend`.

```shell
$ kubectl get deploy backend
NAME      READY   UP-TO-DATE   AVAILABLE   AGE
backend   1/1     1            1           111s
```

An HPA was also created by the `cpuscaler` trait. 

```shell
$ kubectl get HorizontalPodAutoscaler frontend
NAME       REFERENCE             TARGETS         MINPODS   MAXPODS   REPLICAS   AGE
frontend   Deployment/frontend   <unknown>/50%   1         10        1          101m
```