---
title:  How it Works
---

In this documentation, we will explain the core idea of KubeVela and clarify some technical terms that are widely used in the project.

## Overview

First of all, KubeVela introduces a workflow with separate of concerns as below:
- **Platform Team**
  - Model deployment environments and platform capabilities as reusable templates, then register them into Kubernetes.
- **End Users**
  - Choose a deployment environment, assemble the app with available templates per needs, and then deploy the app to target environment.

Below is how this workflow looks like:

![alt](resources/how-it-works.png)

This design make it possible for platform team to enforce best practices by *coding* platform capabilities into templates, and leverage them to expose a *PaaS-like* experience (*i.e. app-centric abstractions, self-service workflow etc*) to end users.

Also, as programmable components, these templates can be updated or extended easily per your needs at any time.

![alt](resources/what-is-kubevela.png)

In the model layer, KubeVela leverages [Open Application Model (OAM)](https://oam.dev) to make above design happen.

## `Application`
The *Application* is the core API of KubeVela. It allows developers to work with a single artifact to capture the complete application deployment with simplified primitives.

In application delivery platform, having an "application" concept is important to simplify administrative tasks and can serve as an anchor to avoid configuration drifts during operation. Also, it provides a much simpler path for on-boarding Kubernetes capabilities to application delivery process without relying on low level details. For example, a developer will be able to model a "web service" without defining a detailed Kubernetes Deployment + Service combo each time, or claim the auto-scaling requirements without referring to the underlying KEDA ScaleObject.

### Example

An example of `website` application with two components (i.e. `frontend` and `backend`) could be modeled as below:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: website
spec:
  components:
    - name: backend
      type: worker
      properties:
        image: busybox
        cmd:
          - sleep
          - '1000'
    - name: frontend
      type: webservice
      properties:
        image: nginx
      traits:
        - type: autoscaler
          properties:
            min: 1
            max: 10
        - type: sidecar
          properties:
            name: "sidecar-test"
            image: "fluentd"
```

The `Application` resource in KubeVela is a LEGO-style object and does not even have fixed schema. Instead, it is composed by building blocks (app components and traits etc.) that allow you to on-board platform capabilities to this application definition via your own abstractions.

The building blocks to model platform capabilities named `ComponentDefinition` and `TraitDefinition`.

### `ComponentDefinition`

`ComponentDefinition` is a pre-defined *template* for the deployable workload. It contains template, parametering and workload characteristic information as a declarative API resource. 

Hence, the `Application` abstraction essentially declares how the user want to **instantiate** given component definitions in target cluster. Specifically, the `.type` field references the name of installed `ComponentDefinition` and `.properties` are the user set values to instantiate it. 

Some typical component definitions are *Long Running Web Service*, *One-time Off Task* or *Redis Database*. All component definitions expected to be pre-installed in the platform, or provided by component providers such as 3rd-party software vendors.

### `TraitDefinition`

Optionally, each component has a `.traits` section that augments the component instance with operational behaviors such as load balancing policy, network ingress routing, auto-scaling policies, or upgrade strategies, etc.

Traits are operational features provided by the platform. To attach a trait to component instance, the user will declare `.type` field to reference the specific `TraitDefinition`, and `.properties` field to set property values of the given trait. Similarly, `TraitDefiniton` also allows you to define *template* for operational features.

We also reference component definitions and trait definitions as *"capability definitions"* in KubeVela. 

## Environment
Before releasing an application to production, it's important to test the code in testing/staging workspaces. In KubeVela, we describe these workspaces as "deployment environments" or "environments" for short. Each environment has its own configuration (e.g., domain, Kubernetes cluster and namespace, configuration data, access control policy, etc.) to allow user to create different deployment environments such as "test" and "production".

Currently, a KubeVela `environment` only maps to a Kubernetes namespace, while the cluster level environment is work in progress.

### Summary

The main concepts of KubeVela could be shown as below:

![alt](resources/concepts.png)

Essentially:
- Components - deployable/provisionable entities that composed your application deployment
  - e.g. a Kubernetes workload, a MySQL database, or a AWS S3 bucket
- Traits - attachable operational features per your needs
  - e.g. autoscaling rules, rollout strategies, ingress rules, sidecars, security policies etc
- Application - full description of your application deployment assembled with components and traits
- Environment - the target environments to deploy this application

## Architecture

The overall architecture of KubeVela is shown as below:

![alt](resources/arch.png)

Specifically, the application controller is responsible for application abstraction and encapsulation (i.e. the controller for `Application` and `Definition`). The rollout controller will handle progressive rollout strategy with the whole application as a unit. The multi-cluster deployment engine is responsible for deploying the application across multiple clusters and environments with traffic shifting and rollout features supported. 
