# Concepts and Glossaries

This document explains some technical terms that are widely used in KubeVela, such as `application`, `appfile`, `workload types` and `traits`. The goal is to clarify them for platform builders in the context of KubeVela.

## Overview

![alt](../resources/concepts.png)

## Application
An application in KubeVela is composed by a collection of components named "services". For instance, a "website" application which is composed of two services: "frontend" and "backend".

Under the hood, KubeVela introduced a Kubernetes [Custom Resource Definition (CRD)](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) named `Application` to capture all needed information to define an app. A simple `application-sample` is as below:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: Application
metadata:
  name: application-sample
spec:
  components: # defines two services in this app
    - name: backend # 1st service
      type: worker 
      settings:
        image: "busybox"
        cmd:
        - sleep
        - "1000"
      traits:
        - name: autoscaler
          properties:
            min: 1
            max: 10
    - name: frontend # 2nd service
      type: webservice
      settings:
        image: "nginx"
```  

### Why Application?

- Provide a single source of truth of the application description.
  - The `Application` object allows developers to work with a single artifact to capture the application definition. It simplifies administrative tasks and also serves as an anchor to avoid configuration drifts during operation. This is extremely useful in application delivery workflow as well as GitOps.
- Lower the learning curve of developers. 
  - The `Application` as a abstraction layer provides a much simpler path for on-boarding Kubernetes capabilities without relying on low level details. For instance, a developer will be able to model the auto-scaling requirements without referring to the underlying [KEDA ScaleObject](https://keda.sh/docs/2.0/concepts/scaling-deployments/#scaledobject-spec).

### Workload & Trait
Each service in the application is modeled by two sections: workload settings and trait properties.

The workload settings section represents the characteristics that runtime infrastructure should take into account to instantiated and deploy this service. Typical workload types including "long running service" and "one-time off task".

The trait properties section represents optional configurations that attaches to an instance of given workload type. Traits augment a workload instance with operational features such as load balancing policy, network ingress routing, circuit breaking, rate limiting, auto-scaling policies, upgrade strategies, and many more.

Note that the schema of both workload settings and trait properties are enforced by modularized capability providers, not by the schema of `Application` CRD. This will be detailed explained in `Capability Modules` section.

## Appfile

KubeVela provided a client-side tool named `Appfile` to help developers design and describe an application with ease. A simple `Appfile` sample is as below:

```yaml
name: testapp

services:
  frontend: # 1st service
    image: oamdev/testapp:v1

    build:
      docker:
        file: Dockerfile
        context: .

    cmd: ["node", "server.js"]
    port: 8080

    route: # a route trait
      domain: example.com
      rules:
        - path: /testapp
          rewriteTarget: /

  backend: # 2nd service
    type: task
    image: perl 
    cmd: ["perl",  "-Mbignum=bpi", "-wle", "print bpi(2000)"]
```

It's by design that `Appfile` is a developer facing tool to render `Application` as well as any other needed Kubernetes resources to ship this app, for example `Secret` and `ConfigMap`. This also means `Appfile` is a superset of `Application`, for example, developers can define a `build` section in `Appfile` which is not part of `Application` CRD.

For full schema of `Appfile`, please check its [ reference documentation](developers/references/devex/appfile.md) for more detail.

## Capability Modules
A capability is a functionality provided by the runtime infrastructure (i.e. Kubernetes) that can support your application management requirements. Both `workload types` and `traits` are common capabilities used in KubeVela.

The capabilities are designed as pluggable modules named "capability definitions", for example, [workload definition](https://github.com/oam-dev/spec/blob/master/4.workload_definitions.md) and [trait definition](https://github.com/oam-dev/spec/blob/master/6.traits.md). KubeVela as the platform builder tool will be responsible for registering, discovering and managing these capabilities following OAM specification.

## Environment
Before releasing an application to production, it's important to test the code in testing/staging workspaces. In KubeVela, we describe these workspaces as "deployment environments" or "environments" for short. Each environment has its own configuration (e.g., domain, Kubernetes cluster and namespace, configuration data, access control policy etc.) to allow user to create different deployment environments such as "test" and "production".

## What's Next

Now that you have grasped the core ideas of KubeVela. Here are some recommended next steps:

- Learn more about KubeVela through its [platform builder guide](platform-engineers/overview.md)
- Continue to try out [end user tutorials](developers/learn-appfile.md) to experience what KubeVela can be used to build