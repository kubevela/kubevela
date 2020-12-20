# Concepts and Glossaries

This document explains some technical terms that are widely used in KubeVela, such as `application`, `service`, `workload type`, `trait` etc., from user's perspective. The goal is to clarify them in the context of KubeVela.

## Overview

![alt](../resources/concepts.png)

## Workload Type & Trait
The core KubeVela APIs are built based on Open Application Model (OAM). Hence, the `workload type` and `trait` concepts are inherited from OAM.

A [workload type](https://github.com/oam-dev/spec/blob/master/4.workload_definitions.md) declares the characteristics that runtime infrastructure should take into account in application management. A typical workload type could be a "long running service", or a "one-time off task" that can be instantiated as part of your application.

A [trait](https://github.com/oam-dev/spec/blob/master/6.traits.md) represents an optional configuration that attaches to an instance of workload type. Traits augment a workload type instance with operational features such as load balancing policy, network ingress routing, circuit breaking, rate limiting, auto-scaling policies, upgrade strategies, and many more.

## Capability
A capability is a functionality provided by the runtime infrastructure (i.e. Kubernetes) that can support your application management requirements. Both `workload type` and `trait` are capabilities defined in KubeVela.

## Service
A service represents the runtime configurations (i.e., workload type and traits) needed to run your application in Kubernetes. Service is the descriptor of a basic deployable unit in KubeVela.

## Application
An application in KubeVela is a collection of services which describes what a developer tries to build and ship from high level. An example could be an "website" application which is composed of two services "frontend" and "backend", or a "wordpress" application which is composed of "php-server" and "database".

An application is defined by an `Appfile` (named `vela.yaml` by default) in KubeVela. Please check its full schema in the [Appfile reference documentation](developers/references/devex/appfile.md).

## Environment
Before releasing an application to production, it's important to test the code in testing/staging workspaces. In KubeVela, we describe these workspaces as "deployment environments" or "environments" for short. Each environment has its own configuration (e.g., domain, Kubernetes namespace, configuration data, access control policy etc.) to allow user to create different deployment environments such as "test" and "production".

## What's Next

Now that you have grasped the core ideas of KubeVela. Here are some recommended next steps:

- Continue to try out [more tutorials](developers/learn-appfile.md)
- Learn how to build platforms with KubeVela following its [platform builder guide](platform-engineers/overview.md)