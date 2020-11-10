# Introduction to KubeVela

![alt](../resources/KubeVela-01.png)

## Motivation

The trend of cloud-native technology is moving towards pursuing consistent application delivery across clouds and on-premises infrastructures using Kubernetes as the common abstraction layer. Kubernetes, although excellent in abstracting low-level infrastructure details, does introduce extra complexity to application developers, namely understanding the concepts of pods, port exposing, privilege escalation, resource claims, CRD, and so on. We’ve seen the nontrivial learning curve and the lack of developer-facing abstraction have impacted user experiences, slowed down productivity, led to unexpected errors or misconfigurations in production. People start to question the value of this revolution: "why am I bothered with these many YAML files?".

On the other hand, abstracting Kubernetes to serve developers' requirements is a highly opinionated process, and the resultant abstractions would only make sense had the decision makers been the platform builders. Unfortunately, the platform builders today face the following dilemma:

*There is no tool or framework for them to easily extend the abstractions if any*. 

Thus, many platforms today introduce restricted abstractions and add-on mechanisms despite the extensibility of Kubernetes. This makes extending such platforms for developers' requirements or to wider scenarios almost impossible, not to mention taking the full advantage of the rich Kubernetes ecosystems.

In the end, developers complain those platforms are too rigid and slow in response to feature requests or improvements. The platform builders do want to help but the engineering effort is daunting: any simple API change in the platform could easily become a marathon negotiation around the opinionated abstraction design.

## What is KubeVela?

For developers, KubeVela itself is an easy-to-use tool that enables them to describe and ship their applications to Kubernetes with minimal effort. Instead of managing a handful Kubernetes YAML files, a simple docker-compose style [Appfile](./docs/developers/devex/appfile.md) is all they need, following an application-centric workflow that can be easily integrated with any CI/CD pipeline.

The above experience cannot be achieved without KubeVela's innovative offerings to the platform builders.

For platform builders, KubeVela serves as a framework that empowers them to create developer facing yet highly extensible platforms at ease. In details, KubeVela relieves the pains of building such platforms by doing the following:

- Application Centric. Behind the Appfile, KubeVela enforces an **application** concept as its main API and **ALL** KubeVela's capabilities serve for the applications' requirements only. This is achieved by adopting the [Open Application Model](https://github.com/oam-dev/spec) as the core API for KubeVela.
 
- Extending Natively. An application in KubeVela is composed of various pluggable workload types and operation features (i.e. traits). Capabilities from Kubernetes ecosystem can be added to KubeVela as new workload types or traits through Kubernetes CRD registry mechanism at any time.

- Simple yet Extensible Abstraction Mechanism. KubeVela's main user interfaces （i.e. Appfile and CLI) are built using a [CUELang](https://github.com/cuelang/cue) based abstraction engine which translates the user-facing schemas to the underline Kubernetes resources. KubeVela provides a set of built-in abstractions to start with and the platform builders are free to modify them at any time. Abstraction changes take effect at runtime, neither recompilation nor redeployment of KubeVela is required.
  
With KubeVela, platform builders now finally have the tooling support to design and ship any new capabilities to their end-users with high confidence and low turn around time. 

## Comparisons

### Platform-as-a-Service (PaaS) 

The typical examples are Heroku and Cloud Foundry. They provides full application management capabilities and aim to improve developer experience and efficiency. KubeVela shares the same goal but its built-in features are much lighter and easier to maintain compared to most of the existing PaaS offerings. KubeVela core components are nothing but a set of Kubernetes controllers/plugins.

Though the biggest difference lies in the extensibility. 

Most PaaS systems enforce constraints in the type of supported applications and the supported capabilities. They are either inextensible or create their own addon systems maintained by the their own communities. In contrast, KubeVela is designed to fully leverage the Kubernetes ecosystem as its capability pool. Hence, there's no additional addon system is introduced in this project. A new capability can be installed in KubeVela at any time by simply registering the CRD and providing a CUElang template.

### Serverless platforms  

Serverless platform such as AWS Lambda provides extraordinary user experience and agility to deploy serverless applications. However, those platforms impose even more constraints in extensibility. They are arguably "hard-coded" PaaS.

Kubernetes based serverless platforms such as Knative, OpenFaaS can be easily integrated with KubeVela by registering themselves as new workload types and traits. Even for AWS Lambda, there is an success story to integrate it with KubeVela by the tools developed by Crossplane.

### Platform agnostic developer tools

The typical example is [Waypoint](https://github.com/hashicorp/waypoint). Waypoint is a developer facing tool which introduces a consistent workflow (i.e., build, deploy, release) to ship applications on top of different platforms.

KubeVela can be integrated into Waypoint like any other supported platforms. In this case, developers will use the Waypoint workflow instead of the KubeVela Appfile/CLI to manage applications, and all the capabilities of KubeVela including abstractions will still be available in this integration.

### Helm, etc. 

Helm is a package manager for Kubernetes that provides package, install, and upgrade a set of YAML files for Kubernetes as a unit. 

KubeVela is not a package manager. Developers are expected to use KubeVela's Appfile to describe how to build and deploy application and that Appfile is not Kubernetes YAML format. However, the server side API object behind Appfile (i.e. `Application Configuration`) is indeed a Kubernetes CRD, it could be packaged and distributed by Helm at ease.

### Kubernetes

KubeVela is a Kubernetes plugin for building application-centric abstractions. It leverages the native Kubernetes extensibility and capabilities to resolve a hard problem - making application management enjoyable on Kubernetes.
