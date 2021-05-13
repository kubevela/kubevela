---
title:  Introduction
slug: / 

---

![alt](resources/KubeVela-01.png)

## Motivation

The trend of cloud-native technology is moving towards pursuing consistent application delivery across clouds and on-premises infrastructures using Kubernetes as the common abstraction layer. Kubernetes, although excellent in abstracting low-level infrastructure details, does introduce extra complexity to application developers, namely understanding the concepts of pods, port exposing, privilege escalation, resource claims, CRD, and so on. We’ve seen the nontrivial learning curve and the lack of developer-facing abstraction have impacted user experiences, slowed down productivity, led to unexpected errors or misconfigurations in production. People start to question the value of this revolution: "why am I bothered with all these details?".

On the other hand, abstracting Kubernetes to serve developers' requirements is a highly opinionated process, and the resultant abstractions would only make sense had the decision makers been the platform team. Unfortunately, the platform team today face the following dilemma:

*There is no tool or framework for them to build user friendly yet highly extensible abstractions for application management*.

Thus, many application platforms today are essentially restricted abstractions with in-house add-on mechanisms despite the extensibility of Kubernetes. This makes extending such platforms for developers' requirements or to wider scenarios almost impossible, not to mention taking the full advantage of the rich Kubernetes ecosystems.

In the end, developers complain those platforms are too rigid and slow in response to feature requests or improvements. The platform team do want to help but the engineering effort is daunting: any simple API change in the platform could easily become a marathon negotiation around the opinionated abstraction design.

## What is KubeVela?

For platform team, KubeVela serves as a framework that relieves the pains of building modern application platforms by doing the following:

**Application Centric** - KubeVela introduces consistent yet application centric API to capture a full deployment of microservices on top of hybrid environments. No infrastructure level concern, simply deploy.

**Natively Extensible** - KubeVela uses CUE to glue capabilities provided by runtime infrastructure and expose them to users via self-service API. When users' needs grow, these API can naturally expand in programmable approach.

**Runtime Agnostic** - KubeVela is built with Kubernetes as control plane but adaptable to any runtime as data-plane. It can deploy (and manage) diverse workload types such as container, cloud functions, databases, or even EC2 instances across hybrid environments.

With KubeVela, the platform team finally have the tooling supports to design easy-to-use application platform with high confidence and low turn around time. 

For end-users (e.g. application team), this platform will enable them to design and ship applications to hybrid environments with minimal effort, and instead of managing a handful infrastructure details, a simple application definition that can be easily integrated with any CI/CD pipeline is all they need.

## Comparisons

### KubeVela vs. Platform-as-a-Service (PaaS) 

The typical examples are Heroku and Cloud Foundry. They provide full application management capabilities and aim to improve developer experience and efficiency. In this context, KubeVela shares the same goal.

Though the biggest difference lies in **flexibility**.

KubeVela enables you to serve end users with programmable building blocks which are fully flexible and coded by yourself. Comparing to this mechanism, traditional PaaS systems are highly restricted, i.e. they have to enforce constraints in the type of supported applications and capabilities, and as application needs grows, you always outgrow the capabilities of the PaaS system - this will never happen in KubeVela platform.

So think of KubeVela as a Heroku that is fully extensible to serve your needs as you grow.

### KubeVela vs. Serverless  

Serverless platform such as AWS Lambda provides extraordinary user experience and agility to deploy serverless applications. However, those platforms impose even more constraints in extensibility. They are arguably "hard-coded" PaaS.

KubeVela can easily deploy Kubernetes based serverless workloads such as Knative, OpenFaaS by referencing them as new components. Even for AWS Lambda, KubeVela can also deploy such workload leveraging Terraform based component.

### KubeVela vs. Platform agnostic developer tools

The typical example is Hashicorp's Waypoint. Waypoint is a developer facing tool which introduces a consistent workflow (i.e., build, deploy, release) to ship applications on top of different platforms.

KubeVela can be integrated with such tools seamlessly. In this case, developers would use the Waypoint workflow as the UI to deploy and manage applications with KubeVela's abstractions (e.g. applications, components, traits etc).

### KubeVela vs. Helm 

Helm is a package manager for Kubernetes that provides package, install, and upgrade a set of YAML files for Kubernetes as a unit. 

KubeVela as a modern deployment system can naturally deploys Helm charts. A common example is you could easily use KubeVela to declare and deploy an application which is composed by a WordPress Helm chart and a AWS RDS instance defined by Terraform, or distribute the Helm chart to multiple clusters.

KubeVela also leverages Helm to manage the capability addons in runtime clusters.

### KubeVela vs. Kubernetes

KubeVela is a Kubernetes add-on for building developer-centric deployment system. It leverages [Open Application Model](https://github.com/oam-dev/spec) and the native Kubernetes extensibility to resolve a hard problem - making shipping applications enjoyable on Kubernetes.

## Getting Started

Now let's [get started](./quick-start) with KubeVela!
