---
title:  Introduction
slug: / 

---

![alt](resources/KubeVela-01.png)

## Motivation

The trend of cloud-native technology is moving towards pursuing consistent application delivery across clouds and on-premises infrastructures using Kubernetes as the common abstraction layer. Kubernetes, although excellent in abstracting low-level infrastructure details, does introduce extra complexity to application developers, namely understanding the concepts of pods, port exposing, privilege escalation, resource claims, CRD, and so on. We’ve seen the nontrivial learning curve and the lack of developer-facing abstraction have impacted user experiences, slowed down productivity, led to unexpected errors or misconfigurations in production. People start to question the value of this revolution: "why am I bothered with all these details?".

On the other hand, abstracting Kubernetes to serve developers' requirements is a highly opinionated process, and the resultant abstractions would only make sense had the decision makers been the platform builders. Unfortunately, the platform builders today face the following dilemma:

*There is no tool or framework for them to easily build user friendly yet highly extensible abstractions*. 

Thus, many platforms today are essentially restricted abstractions with in-house add-on mechanisms despite the extensibility of Kubernetes. This makes extending such platforms for developers' requirements or to wider scenarios almost impossible, not to mention taking the full advantage of the rich Kubernetes ecosystems.

In the end, developers complain those platforms are too rigid and slow in response to feature requests or improvements. The platform builders do want to help but the engineering effort is daunting: any simple API change in the platform could easily become a marathon negotiation around the opinionated abstraction design.

## What is KubeVela?

For platform builders, KubeVela serves as a framework that relieves the pains of building developer focused platforms by doing the following:

- Developer Centric. KubeVela introduces the Application as the main API to capture a full deployment of microservices, and builds features around the application needs only. Progressive rollout and multi-cluster deployment are provided out-of-box. No infrastructure level concerns, simply deploy.
 
- Extending Natively. In KubeVela, all platform features (such as workloads, operational behaviors, and cloud services) are defined as reusable [CUE](https://github.com/cuelang/cue) and/or [Helm](https://helm.sh) components, per needs of the application deployment. And when application's needs grow, your platform capabilities expand naturally in a programmable approach.

- Simple yet Reliable. Perfect in flexibility though, X-as-Code may lead to configuration drift (i.e. the running instances are not in line with the expected configuration). KubeVela solves this by modeling its capabilities as code but enforce them via Kubernetes control loop which will never leave inconsistency in your clusters. This also makes KubeVela work with any CI/CD or GitOps tools via declarative API without integration burden.

With KubeVela, the platform builders finally have the tooling supports to design easy-to-use abstractions and ship them to end-users with high confidence and low turn around time. 

For end-users (e.g. app developers and operators), these abstractions will enable them to design and ship applications to Kubernetes clusters with minimal effort, and instead of managing a handful infrastructure details, a simple application definition that can be easily integrated with any CI/CD pipeline is all they need.

## Comparisons

### KubeVela vs. Platform-as-a-Service (PaaS) 

The typical examples are Heroku and Cloud Foundry. They provide full application management capabilities and aim to improve developer experience and efficiency. In this context, KubeVela shares the same goal.

Though the biggest difference lies in **flexibility**.

KubeVela is a Kubernetes add-on that enabling you to serve end users with programmable building blocks which are fully flexible and coded by yourself. Comparing to this mechanism, traditional PaaS systems are highly restricted, i.e. they have to enforce constraints in the type of supported applications and capabilities, and as application needs grows, you always outgrow the capabilities of the PaaS system - this will never happen in KubeVela platform.

So think of KubeVela as a Heroku that is fully extensible to serve your needs as you grow.

### KubeVela vs. Serverless  

Serverless platform such as AWS Lambda provides extraordinary user experience and agility to deploy serverless applications. However, those platforms impose even more constraints in extensibility. They are arguably "hard-coded" PaaS.

Kubernetes based serverless platforms such as Knative, OpenFaaS can be easily integrated with KubeVela by registering themselves as new workload types and traits. Even for AWS Lambda, there is a success story to integrate it with KubeVela by the tools developed by Crossplane.

### KubeVela vs. Platform agnostic developer tools

The typical example is Hashicorp's Waypoint. Waypoint is a developer facing tool which introduces a consistent workflow (i.e., build, deploy, release) to ship applications on top of different platforms.

KubeVela can be integrated into such tools as an application platform. In this case, developers could use the Waypoint workflow to manage applications with leverage of abstractions (e.g. application, rollout, ingress, autoscaling etc) you built via KubeVela.

### KubeVela vs. Helm 

Helm is a package manager for Kubernetes that provides package, install, and upgrade a set of YAML files for Kubernetes as a unit. KubeVela can patch, deploy and rollout Helm packaged application components, and it also leverages Helm to manage the capability dependencies in system level.

Though KubeVela itself is not a package manager, it's a core engine for platform builders to create developer-centric deployment system with easy and repeatable approach.

### KubeVela vs. Kubernetes

KubeVela is a Kubernetes add-on for building developer-centric deployment system. It leverages [Open Application Model](https://github.com/oam-dev/spec) and the native Kubernetes extensibility to resolve a hard problem - making shipping applications enjoyable on Kubernetes.

## Getting Started

Now let's [get started](./quick-start) with KubeVela!
