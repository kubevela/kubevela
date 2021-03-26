# Introduction to KubeVela

![alt](../resources/KubeVela-01.png)

## Motivation

The trend of cloud-native technology is moving towards pursuing consistent application delivery across clouds and on-premises infrastructures using Kubernetes as the common abstraction layer. Kubernetes, although excellent in abstracting low-level infrastructure details, does introduce extra complexity to application developers, namely understanding the concepts of pods, port exposing, privilege escalation, resource claims, CRD, and so on. We’ve seen the nontrivial learning curve and the lack of developer-facing abstraction have impacted user experiences, slowed down productivity, led to unexpected errors or misconfigurations in production. People start to question the value of this revolution: "why am I bothered with all these details?".

On the other hand, abstracting Kubernetes to serve developers' requirements is a highly opinionated process, and the resultant abstractions would only make sense had the decision makers been the platform builders. Unfortunately, the platform builders today face the following dilemma:

*There is no tool or framework for them to easily build user friendly yet highly extensible abstractions*. 

Thus, many platforms today are essentially restricted abstractions with in-house add-on mechanisms despite the extensibility of Kubernetes. This makes extending such platforms for developers' requirements or to wider scenarios almost impossible, not to mention taking the full advantage of the rich Kubernetes ecosystems.

In the end, developers complain those platforms are too rigid and slow in response to feature requests or improvements. The platform builders do want to help but the engineering effort is daunting: any simple API change in the platform could easily become a marathon negotiation around the opinionated abstraction design.

## What is KubeVela?

For platform builders, KubeVela serves as a framework that empowers them to create user friendly yet highly extensible platforms at ease. In details, KubeVela relieves the pains of building such platforms by doing the following:

- Application Centric. KubeVela enforces an *Application* abstraction as its main API and **ALL** KubeVela's capabilities serve for the applications' needs only. This is achieved by adopting the [Open Application Model](https://github.com/oam-dev/spec) as the core API for KubeVela.
 
- Extending Natively. The *Application* abstraction is composed of modularized building blocks named *components* and *traits*. Any capability provided by Kubernetes ecosystem can be added to KubeVela as new component or trait through simple `kubectl apply -f`.

- Simple yet Extensible Abstraction Mechanism. The *Application* abstraction is implemented with server-side encapsulation controller (supports [CUELang](https://github.com/cuelang/cue) and [Helm](https://helm.sh) as templating engine) to abstract user-facing primitives from Kubernetes API resources. Changes to existing capability templates (or new templates added) take effect at runtime, neither recompilation nor redeployment of KubeVela is required.

With KubeVela, platform builders now finally have the tooling support to design and ship any new capabilities to their end-users with high confidence and low turn around time. 

For developers, such *Application* abstraction built with KubeVela will enable them to design and ship their applications to Kubernetes with minimal effort. Instead of managing a handful infrastructure details, a simple application definition that can be easily integrated with any CI/CD pipeline is all they need.

## Comparisons

### Platform-as-a-Service (PaaS) 

The typical examples are Heroku and Cloud Foundry. They provides full application management capabilities and aim to improve developer experience and efficiency. In this context, KubeVela can provide similar experience but its built-in features are much lighter and easier to maintain compared to most of the existing PaaS offerings. KubeVela core components are nothing but a set of Kubernetes controllers/plugins.

Though the biggest difference lies KubeVela positions itself as the engine to build "PaaS-like" systems, not a PaaS offering.

KubeVela is designed as a core engine whose primary goal is to enable platform team to create "PaaS-like" experience by simply registering Kubernetes API resources and defining templates. Comparing to this experience, most existing PaaS systems are either inextensible or have their own addon systems. Hence it's common for them to enforce constraints in the type of supported applications and the supported capabilities which will not happen in KubeVela based experience. 

### Serverless platforms  

Serverless platform such as AWS Lambda provides extraordinary user experience and agility to deploy serverless applications. However, those platforms impose even more constraints in extensibility. They are arguably "hard-coded" PaaS.

Kubernetes based serverless platforms such as Knative, OpenFaaS can be easily integrated with KubeVela by registering themselves as new workload types and traits. Even for AWS Lambda, there is an success story to integrate it with KubeVela by the tools developed by Crossplane.

### Platform agnostic developer tools

The typical example is Hashicorp's Waypoint. Waypoint is a developer facing tool which introduces a consistent workflow (i.e., build, deploy, release) to ship applications on top of different platforms.

KubeVela can be integrated into Waypoint like any other supported platforms. In this case, developers will use the Waypoint workflow to manage applications, and all the capabilities of KubeVela including abstractions will still be available in this integration.

### Helm, etc. 

Helm is a package manager for Kubernetes that provides package, install, and upgrade a set of YAML files for Kubernetes as a unit. KubeVela leverages Helm heavily to package the capability dependencies and Helm controller is one of the core components behind *Application* abstraction.

Though KubeVela itself is not a package manager, it's a core engine for platform builders to create upper layer platforms in easy and repeatable approach.

### Kubernetes

KubeVela is a Kubernetes plugin for building upper layer platforms. It leverages the native Kubernetes extensibility and capabilities to resolve a hard problem - making shipping applications enjoyable on Kubernetes.

## Getting Started

[Install KubeVela](./install.md) into any Kubernetes cluster to get started.