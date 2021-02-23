# Core Concepts

In this documentation, we will explain the core idea of KubeVela and clarify some technical terms that are widely used in the project.

## Separate of Concerns

First of all, KubeVela introduces a workflow with separate of concerns as below:
- **Platform**
  - Responsibility: defining templates for deployment environments and reusable modules such as component types, operational behaviors, and registering them into the cluster.
  - Example: *infrastructure operators, platform builders*.
- **End Users**
  - Responsibility: choose a deployment environment, model and assemble the app with available modules, and deploy the app to target environment.
  - Example: *app developers, app operators*.

![alt](../resources/how-it-works.png)

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

For each of the components in `Application`, its `.type` field represents the runtime characteristic of this component (i.e. workload type) and `.settings` claims the configurations to instantiate it. Some typical component types are *Long Running Web Service*, *One-time Off Task* or *Redis Database*.

All supported component types expected to be pre-installed in the platform, or, provided by component providers such as 3rd-party software owner.

### Traits

Optionally, each component has a `.traits` section that augments its component instance with operational behaviors such as load balancing policy, network ingress routing, auto-scaling policies, or upgrade strategies, etc. Its `.name` field references the specific trait definition, and `.properties` sets detailed configuration values of the given trait.

The supported traits are mostly operational features provided by the platform, but the platform also allows users bring their own traits.

We also reference workload type and trait as "application/capability modules" in KubeVela.

## Definitions

Both the schemas of workload settings and trait properties in `Application` are enforced by application modules that are pre-defined separately in a set of definition objects. The platform teams or component providers are responsible for registering and managing definitions in target cluster following [workload definition](https://github.com/oam-dev/spec/blob/master/4.workload_definitions.md) and [trait definition](https://github.com/oam-dev/spec/blob/master/6.traits.md) specifications in Open Application Model (OAM). 

Currently, KubeVela supports [CUE](https://github.com/cuelang/cue) as the module type in definitions, with more types such as Helm and Terraform are coming in following releases. In the case of Helm, a chart could be directly referenced as a component type in `Application` and its `values.yaml` will become the component's specification.

## Environment
Before releasing an application to production, it's important to test the code in testing/staging workspaces. In KubeVela, we describe these workspaces as "deployment environments" or "environments" for short. Each environment has its own configuration (e.g., domain, Kubernetes cluster and namespace, configuration data, access control policy etc.) to allow user to create different deployment environments such as "test" and "production".

Currently, a KubeVela `environment` only maps to a Kubernetes namespace, while the cluster level environment is work in progress.

### Summary

The main concepts of KubeVela could be shown as below:

![alt](../resources/concepts.png)

## Architecture

The overall architecture of KubeVela is shown as below:

![alt](../../resources/kubevela-runtime.png)

The encapsulation engine in KubeVela is responsible for application abstraction and encapsulation (i.e. the controller for `Application` and `Definition`).

The deployment engine (*currently WIP*) is responsible for progressive rollout of the application (i.e. the controller for `AppDeployment`).


## What's Next

Now that you have grasped the core ideas of KubeVela, let's learn more details about [Application Encapsulation and Abstraction](platform-engineers/overview.md) feature.
