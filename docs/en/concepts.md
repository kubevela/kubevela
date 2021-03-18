# Core Concepts

*"KubeVela is a scalable way to create PaaS-like experience on Kubernetes"*

In this documentation, we will explain the core idea of KubeVela and clarify some technical terms that are widely used in the project.

## Overview

First of all, KubeVela introduces a workflow with separate of concerns as below:
- **Platform Team**
  - Defining templates for deployment environments and reusable capability modules to compose an application, and registering them into the cluster.
- **End Users**
  - Choose a deployment environment, model and assemble the app with available modules, and deploy the app to target environment.

Below is how this workflow looks like:

![alt](../resources/how-it-works.png)

This template based workflow make it possible for platform team enforce best practices and deployment confidence with a set of Kubernetes CRDs, and give end users a *PaaS-like* experience (*i.e. app-centric, higher level abstractions, self-service operations etc*) by natural.

![alt](../resources/what-is-kubevela.png)

Below are the core building blocks in KubeVela that make this happen.

## Application
The *Application* is the core API of KubeVela. Its main purpose is for **application encapsulation and abstraction**, i.e. it allows developers to work with a single artifact to capture the complete application definition with simplified primitives.

Application encapsulation is important to simplify administrative tasks and can serve as an anchor to avoid configuration drifts during operation. Also, as an abstraction object, `Application` provided a much simpler path for on-boarding Kubernetes capabilities without relying on low level details. For example, a developer will be able to model a "web service" without defining detailed Kubernetes Deployment + Service combo each time, or claim the auto-scaling requirements without referring to the underlying KEDA ScaleObject.

An example of `website` application with two components (i.e. `frontend` and `backend`) could be modeled as below:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: Application
metadata:
  name: website
spec:
  components:
    - name: backend
      type: worker
      settings:
        image: busybox
        cmd:
          - sleep
          - '1000'
    - name: frontend
      type: webservice
      settings:
        image: nginx
      traits:
        - name: autoscaler
          properties:
            min: 1
            max: 10
        - name: sidecar
          properties:
            name: "sidecar-test"
            image: "fluentd"
```

### Workload Types

For each of the components in `Application`, its `.type` field represents the definition of this component type and `.settings` claims the values to instantiate it. Some typical component types are *Long Running Web Service*, *One-time Off Task* or *Redis Database*.

All supported component types expected to be pre-installed in the platform, or, provided by component providers such as 3rd-party software owner.

### Traits

Optionally, each component has a `.traits` section that augments its component instance with operational behaviors such as load balancing policy, network ingress routing, auto-scaling policies, or upgrade strategies, etc.

Essentially, traits are operational features provided by the platform, note that KubeVela allows users bring their own traits as well. To attach a trait, use `.name` field to reference the specific trait definition, and `.properties` field to set detailed configuration values of the given trait.

We also reference component types and traits as *"capabilities"* in KubeVela. 

## Definitions

Both the schemas of workload settings and trait properties in `Application` are enforced by a set of definition objects. The platform teams or component providers are responsible for registering and managing definition objects in target cluster following [workload definition](https://github.com/oam-dev/spec/blob/master/4.workload_types.md) and [trait definition](https://github.com/oam-dev/spec/blob/master/6.traits.md) specifications in Open Application Model (OAM). 

Specifically, definition object carries the templating information of this capability. Currently, KubeVela supports [Helm](http://helm.sh/) charts and [CUE](https://github.com/cuelang/cue) modules as definitions which means you could use KubeVela to deploy Helm charts and CUE modules as application components, or claim them as traits. More capability types support such as [Terraform](https://www.terraform.io/) is also work in progress.

## Environment
Before releasing an application to production, it's important to test the code in testing/staging workspaces. In KubeVela, we describe these workspaces as "deployment environments" or "environments" for short. Each environment has its own configuration (e.g., domain, Kubernetes cluster and namespace, configuration data, access control policy etc.) to allow user to create different deployment environments such as "test" and "production".

Currently, a KubeVela `environment` only maps to a Kubernetes namespace, while the cluster level environment is work in progress.

### Summary

The main concepts of KubeVela could be shown as below:

![alt](../resources/concepts.png)

## Architecture

The overall architecture of KubeVela is shown as below:

![alt](../../resources/arch.png)

Specifically, the application controller is responsible for application abstraction and encapsulation (i.e. the controller for `Application` and `Definition`). The rollout controller will handle progressive rollout strategy with the whole application as a unit. The multi-env deployment engine (*currently WIP*) is responsible for deploying the application across multiple clusters and environments. 


## What's Next

Now that you have grasped the core ideas of KubeVela, let's learn more details about [Application CRD](platform-engineers/overview.md) feature.
