---
title:  Introduction
slug: / 

---

![alt](../resources/KubeVela-01.png)

## Motivation

The trend of cloud-native technology is moving towards pursuing consistent application delivery across clouds and on-premises infrastructures using Kubernetes as the common abstraction layer. Kubernetes, although excellent in abstracting low-level infrastructure details, does introduce extra complexity to application developers, namely understanding the concepts of pods, port exposing, privilege escalation, resource claims, CRD, and so on. We’ve seen the nontrivial learning curve and the lack of developer-facing abstraction have impacted user experiences, slowed down productivity, led to unexpected errors or misconfigurations in production. People start to question the value of this revolution: "why am I bothered with all these details?".

On the other hand, abstracting Kubernetes to serve developers' requirements is a highly opinionated process, and the resultant abstractions would only make sense had the decision makers been the platform builders. Unfortunately, the platform builders today face the following dilemma:

*There is no tool or framework for them to easily build user friendly yet highly extensible abstractions*. 

Thus, many platforms today are essentially restricted abstractions with in-house add-on mechanisms despite the extensibility of Kubernetes. This makes extending such platforms for developers' requirements or to wider scenarios almost impossible, not to mention taking the full advantage of the rich Kubernetes ecosystems.

In the end, developers complain those platforms are too rigid and slow in response to feature requests or improvements. The platform builders do want to help but the engineering effort is daunting: any simple API change in the platform could easily become a marathon negotiation around the opinionated abstraction design.

## What is KubeVela?

For platform builders, KubeVela serves as a framework that relieves the pains of building developer focused platforms by doing the following:

- Developer Centric. KubeVela abstracts away the infrastructure level primitives by introducing the *Application* concept as main API, and then building operational features around the applications' needs only.
 
- Extending Natively. The *Application* is composed of modularized building blocks that support [CUELang](https://github.com/cuelang/cue) and [Helm](https://helm.sh) as template engines. This enable you to abstract Kubernetes capabilities in LEGO-style and ship them to end users via plain `kubectl apply -f`. Changes made to the abstraction templates take effect at runtime, neither recompilation nor redeployment of KubeVela is required.

- Simple yet Reliable Abstraction Mechanism. Unlike most IaC (Infrastructure-as-Code) solutions, the abstractions in KubeVela is built with [Kubernetes Control Loop](https://kubernetes.io/docs/concepts/architecture/controller/) so they will never leave *Configuration Drift* in your cluster. As a [Kubernetes Custom Resource](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/), KubeVela works with any CI/CD or GitOps tools seamlessly, no integration effort needed.

With KubeVela, the platform builders finally have the tooling supports to design easy-to-use abstractions and ship them to end-users with high confidence and low turn around time. 

For end-users (e.g. app developers), such abstractions built with KubeVela will enable them to design and ship applications to Kubernetes with minimal effort - instead of managing a handful infrastructure details, a simple application definition that can be easily integrated with any CI/CD pipeline is all they need.

## Comparisons

### KubeVela vs. Platform-as-a-Service (PaaS) 

The typical examples are Heroku and Cloud Foundry. They provide full application management capabilities and aim to improve developer experience and efficiency. In this context, KubeVela can provide similar experience.

Though the biggest difference lies in **flexibility**.

KubeVela is a Kubernetes plug-in that enabling you to serve end users with simplicity by defining your own abstractions, and this is achieved by templating Kubernetes API resources as application-centric abstractions in your cluster. Comparing to this mechanism, most existing PaaS systems are highly restricted and inflexible, i.e. they have to enforce constraints in the type of supported applications and capabilities, and as application needs grows, they always outgrow the capabilities of a PaaS system - this will never happen in KubeVela. 

### KubeVela vs. Serverless  

Serverless platform such as AWS Lambda provides extraordinary user experience and agility to deploy serverless applications. However, those platforms impose even more constraints in extensibility. They are arguably "hard-coded" PaaS.

Kubernetes based serverless platforms such as Knative, OpenFaaS can be easily integrated with KubeVela by registering themselves as new workload types and traits. Even for AWS Lambda, there is an success story to integrate it with KubeVela by the tools developed by Crossplane.

### KubeVela vs. Platform agnostic developer tools

The typical example is Hashicorp's Waypoint. Waypoint is a developer facing tool which introduces a consistent workflow (i.e., build, deploy, release) to ship applications on top of different platforms.

KubeVela can be integrated into Waypoint as a supported platform. In this case, developers could use the Waypoint workflow to manage applications with leverage of abstractions (e.g. application, rollout, ingress, autoscaling etc) you built via KubeVela.

### KubeVela vs. Helm 

Helm is a package manager for Kubernetes that provides package, install, and upgrade a set of YAML files for Kubernetes as a unit. KubeVela leverages Helm heavily to package the capability dependencies and Helm is also one of the core templating engines behind *Application* abstraction.

Though KubeVela itself is not a package manager, it's a core engine for platform builders to create upper layer platforms in easy and repeatable approach.

### KubeVela vs. Kubernetes

KubeVela is a Kubernetes plugin for building higher level abstractions. It leverages [Open Application Model](https://github.com/oam-dev/spec) and the native Kubernetes extensibility to resolve a hard problem - making shipping applications enjoyable on Kubernetes.

## Getting Started

Now let's [get started](./quick-start.md) with KubeVela!
