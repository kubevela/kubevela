# KubeVela Design

This document is the detailed design and architecture of the KubeVela being built in this repository.

> All the diagram in this documentation could be found in [this slides](https://docs.google.com/presentation/d/1Y3gnKrd7fUZGgee7Ia9vBsRIYhcZLQwMUCDkk1RJvQc/edit?usp=sharing).

## Overview

KubeVela is a simple, complete, but highly extensible cloud native application platform based on Kubernetes and Open Application Model (OAM). KubeVela intends to bring application-centric experience to its end users and democratize building cloud native application platforms for platform engineers.

## User Stories

As a end user (e.g. application developers, operators etc) of the platform, I only want to focus on coding my business logic and ship them to various environments at ease. Let's say:
- Here's my source code.
- Here's my application configuration (described in end user PoV).
- Deploy it in test environment.
- Deploy it in production environment.
- Monitoring, debugging, rollout/rollback the application.
- Dockerfile is fine, but please keep it simple. 

As a platform engineer, I want to build a easy-to-use platform for my end users. In detail, the platform should be:
- Heroku-like (_in terms of both user experience and functionality_). 
  - and I prefer to build my own version with OSS tools, particularly, with Kubernetes (for obvious reason).
- Easy to build. 
  - I am too busy to reinvent any wheel, I want to reuse every capability in Kubernetes community as possible,  with minimal effort. Writing some simple CRD and controllers is fine, but please, just the simple ones like copy-paste.
- Powerful and highly extensible.
  - I don't want to lock my users with restricted abstractions and capabilities like traditional PaaS or FaaS/Serverless. I love Kubernetes and what it has enabled. So in terms of capability, I hope my platform is fully open and has unlimited possibilities like Kubernetes itself, rather than another opinionated close system like traditional PaaS.


## Core Principles

In nutshell, the principles for KubeVela project are:

- For end users, it out-of-the-box ships a fully featured PaaS-like experience, nothing special.
- For platform builders, it works like a special Kubernetes "distro" or extensible PaaS core that could be used to build something more complex. It allows platform builders to ship any existing capabilities in cloud native ecosystem to end users with minimal effort, or develop a new capability at ease in a standardized and Kubernetes-native approach.

## Design Details

### 1. Application Centric

The design of KubeVela intends to make users think in terms of application, not containers or infrastructure. 

Lacking application context impacts the user experience and significantly raised the bar to adopt cloud native stack. We believe "application" is the natural mindset of developers and it's the core concept an application platform should expose.

![alt](resources/app-centric.png)

KubeVela intends to let developers push code, define application in developer facing primitives, and make daily operations as configurations of the application.  

Thus, KubeVela choose to:
1. Introduce "application" as first class citizen and main API.
2. Build the whole system around "application", i.e. model capabilities of Kubernetes as application configuration, with clarity and manageability.

#### Solution

Instead of creating a in-house "application CRD", KubeVela adopts [Open Application Model (OAM)](https://github.com/oam-dev/spec) as its application definition, since OAM:
1. Defines micro-services application by default.
2. Can model operations as part of the application (i.e. `Trait`).
2. Highly extensible: every workload and trait in OAM is a independent definition, no abstraction or capability lock-in.

### 2. Capability Oriented Architecture

To enable platform builders use KubeVela as the extensible "PaaS core", KubeVela intends to make its every capability a standalone "plug-in".

![alt](resources/coa.png)

For example, there are several "built-in" workload types in KubeVela such as `Web Service` or `Task`. It is by design that they are all independent CRD controllers that abstract Kubernetes built-in workloads and create Service automatically if needed. KubeVela itself is **NOT** aware of the specification or implementation of these workload types.

This means platform builders are free to bring their own workload types by simply install a CRD controller, or even just reference a k8s built-in resource like StatefulSet as new workload type.

Similarly, all the "built-in" operations such as `scaling` or `rollout` (i.e. "traits" in KubeVela) are also independent CRD controllers which are **NOT** bound with specific workload types. Platform builders are free to bring their own traits implementations by simply providing a CRD controller, reference a k8s built-in resource like `HPA` or `NetworkPolicy` as trait is also possible.

This loosely coupled design of KubeVela adopts the idea of Capability Oriented Architecture (COA), i.e. instead of creating a close system like traditional PaaS, KubeVela intends to become an application-centric framework to connect end users with underlying infrastructure capabilities.

#### Solution

We decide to build KubeVela core with [OAM Kubernetes runtime](https://github.com/crossplane/oam-kubernetes-runtime) which already implemented the building block features such as standalone workload type and trait controllers. Whether a given trait could work with certain workload types is also clarified as `appliesTo` field of trait definition. For platform builders, OAM Kubernetes Runtime also provided a [lightweight library](https://github.com/crossplane/oam-kubernetes-runtime/blob/2be3900a3817aed570de9ec353e6ab0b50e100f0/pkg/controller/v1alpha2/core/traits/manualscalertrait/manualscalertrait_controller.go#L42) to implement trait controller so it doesn't need to care about workload kinds.

##### Capability Register and Discovery

KubeVela leverages [OAM definition objects](https://github.com/oam-dev/spec/blob/master/4.workload_definitions.md) to register and discover workloads and traits:


```console
$ kubectl apply -f workload-definition.yaml # register a new workload type
$ kubectl apply -f trait-definition.yaml # register a new trait
```

Note that OAM definition objects only care about API resource, not including the controllers. Thus KubeVela intends to include a **CRD registry** so whenever a new API resource is installed as workload or trait, KubeVela could install its controller automatically from the registry. That of course means we envision the CRD registry could register a CRD and Helm chart (which contains the manifest of the controller). In practice, we are currently evaluating RedHat's Operator Lifecycle Manager (OLM) but no the final conclusion yet.

##### Cloud Services Integration

For capabilities like cloud services, KubeVela intends to leverage Kubernetes as the universal control plane so [Crossplane](https://github.com/crossplane/crossplane) core will be used to register cloud services as workload types.


### 3. Extensible User Interface

KubeVela is built with Kubernetes and OAM also adopts Kubernetes API resource model. So in nutshell, **ALL** functionalities of KubeVela core are able to be consumed by simple `kubectl`, for example:

```yaml
$ kubectl apply -f frontend-component.yaml # create frontend component
$ kubectl apply -f backend-component.yaml # create backend component
$ kubectl apply -f application-config.yaml # assign operational traits to components
```

However, Kubernetes API should not be the end game. Declarative YAML objects are great to build platforms like KubeVela with but when directly exposed to end users, it creates heavy mental burden and high learning curve. For example, even with highest level abstraction, the default [containerized workload schema](https://github.com/oam-dev/spec/blob/master/core/workloads/schema/containerized_workload.md) of OAM still has dozens fields to learn with many rough edges to be aware.

Thus KubeVela intends to introduce a lightweight user facing layer on top of its Kubernetes API objects with following goals in mind:

- Shorten the learning curve of new developers. Most capabilities in KubeVela are developed by big
companies that run very complex workloads. However, for the bigger developer community, the new user facing layer will provide a much simpler path to on-board KubeVela.
- Developers can describe their applications and behavior of their components without making assumptions on availability of specific Kubernetes API. For instance, a developer will be able to model auto-scaling needs without referring to the CRD of auto-scaling trait.
- Provides a single source of truth of the application description. The user facing layer allows developers to work with a single artifact to capture the application definition. This artifact is the definitive truth of how the application is supposed to look like. It simplifies administrative tasks such as change management. It also serves as an anchor for application truth to avoid configuration drifts during operation.
- Highly extensible. For example, when a new workload type or trait is installed, the end users could access this new capability directly from user interface layer, no re-compile or re-deploy of KubeVela is required.

#### Solution

We concluded above requirements of the user interface layer as introducing a modeling language on top of the existing model objects. With this context, we decide to adopt [CUElang](https://github.com/cuelang/cue) as the modeling language of KubeVela. In detail, every workload or trait definition will inline a cue template as the contract between user and Kubernetes API object.

However, we don't assume users need to learn or write CUElang directly. Instead, the reason we choose CUElang is it's just perfect to help us create developer facing tools based on it. Thus, KubeVela introduced three most common tools based to CUElang for developers to use:

1. A command line tool.
2. A GUI dashboard.
3. A Docker Compose style `appfile`.

An example for `vela cli`:

```console
$ vela comp deploy frontend -t webservice --image oamdev/testapp:v1 --port 80 --app helloworld
```

The `-t webservice --image oamdev/testapp:v1 --port 80` arguments are not hard coded, they are schema defined by in-line CUE template of `WebService` workload definition.

The `appfile` is essentially a YAML version of command line tool so we can support more complex and serious scenarios by simply running `$ vela up hello-world.yaml`:

```yaml
version: "1.0-alpha.1"

name: helloworld

services:
  express-server:
    type: webservice # workload type
    build:
      docker:
        file: Dockerfile
        context: .

    image: oamdev/testapp:v1
    cmd: ["node", "server.js"]
    ports:
      - 8080:80
    env:
      - FOO=bar

    scale: # scaling trait
      replica: 2
      auto:
        range: "1-10"
        cpu: 80
        qps: 1000

    canary: # canary trait
      step: 5
      headers:
        - "foo:bar.*"

  redis:
    image: oamdev/redis

secrets:
  my-secret: /local-path/my-secret # load local file into k8s secret
```

The schema of above `appfile` is not hard coded, they are defined by in-line CUE templates of `WebService` workload definition, `Scaling` trait definition and `Canary` trait definition.

We will skip the example of dashboard, but similarly, the schema of GUI forms are defined by in-lined CUE template of definition objects.

## Architecture

![alt](resources/arch.png)

From highest level, KubeVela is composed by only two components:

### 1. User interface layer
Including: `cli`, `dashboard`, `appfile`, they are all client side tools to provide developer facing abstractions by leveraging CUElang based parametering and templating.
### 2. KubeVela core
Including:
  - [OAM Kubernetes runtime](https://github.com/crossplane/oam-kubernetes-runtime) to provide application level building blocks such as `Component` and `Application` etc.
  - [Built-in workload and trait controllers](https://github.com/oam-dev/kubevela/tree/master/pkg/controller/v1alpha1) to implement core capabilities such as `webservice`, `route` and `rollout` etc.
  - Capability Management: manage features of KubeVela following Capability Oriented Architecture. 
    - Every feature of KubeVela is a "addon", and it is registered by Kubernetes API resource (including CRD) leveraging OAM definition objects.
    - CRD Registry: register controllers of Kubernetes add-ons and discover them by CRD. This will enable automatically install controllers/operators when CRD is missing in the cluster.
