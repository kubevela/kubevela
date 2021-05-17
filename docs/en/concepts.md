---
title:  How it Works
---

In this documentation, we will explain the core idea of KubeVela and clarify some technical terms that are widely used in the project.

## Architecture

The overall architecture of KubeVela is shown as below:

![alt](resources/arch.png)

### Control Plane Cluster

The main part of KubeVela. It is the component that users will interact with and where the KubeVela controllers are running on. Control plane cluster will deploy the application to multiple *runtime clusters*. 

### Runtime Clusters

Runtime clusters are where the applications are actually running on. The needed addons in runtime cluster are automatically discovered and installed with leverage of [CRD Lifecycle Management (CLM)](https://github.com/cloudnativeapp/CLM).


## API

On control plane cluster, KubeVela introduces [Open Application Model (OAM)](https://oam.dev) as the higher level API. Hence users of KubeVela only work on application level with a consistent experience, regardless of the complexity of heterogeneous runtime environments.

This API could be explained as below:

![alt](resources/concepts.png)

In detail:
- Components - deployable/provisionable entities that compose your application.
  - e.g. a Helm chart, a Kubernetes workload, a CUE or Terraform module, or a cloud database instance etc.
- Traits - attachable features that will *overlay* given component with operational behaviors.
  - e.g. autoscaling rules, rollout strategies, ingress rules, sidecars, security policies etc.
- Application - full description of your application deployment assembled with components and traits.
- Environment - the target environments to deploy this application.

We also reference components and traits as *"capabilities"* in KubeVela.

## Workflow

To ensure simple yet consistent user experience across hybrid environments. KubeVela introduces a workflow with separate of concerns as below:
- **Platform Team**
  - Model and manage platform capabilities as components or traits, together with target environments specifications.
- **Application Team**
  - Choose a environment, assemble the application with components and traits per needs, and deploy it to target environment.

> Note that either platform team or application team application will only talk to the control plane cluster. KubeVela is designed to hide the details of runtime clusters except for debugging or verifying purpose.

Below is how this workflow looks like:

![alt](resources/how-it-works.png)

All the capability building blocks can be updated or extended easily at any time since they are fully programmable (currently support CUE, Terraform and Helm).

## Environment
Before releasing an application to production, it's important to test the code in testing/staging workspaces. In KubeVela, we describe these workspaces as "environments". Each environment has its own configuration (e.g., domain, Kubernetes cluster and namespace, configuration data, access control policy, etc.) to allow user to create different deployment environments such as "test" and "production".

Currently, a KubeVela `environment` only maps to a Kubernetes namespace. In the future, a `environment` will contain multiple clusters.

## What's Next

Here are some recommended next steps:

- Learn how to [deploy an application](end-user/application) and understand how it works.
- Join `#kubevela` channel in CNCF [Slack](https://cloud-native.slack.com) and/or [Gitter](https://gitter.im/oam-dev/community)

Welcome onboard and sail Vela!
