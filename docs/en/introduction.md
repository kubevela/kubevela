---
title:  Introduction
slug: / 

---

![alt](resources/KubeVela-01.png)

## Motivation

The trend of cloud-native technology is moving towards pursuing consistent application delivery across clouds and on-premises infrastructures using Kubernetes as the common layer. Kubernetes, although excellent in abstracting low-level infrastructure details, does not introduce abstractions to model software deployment on top of today's hybrid and distributed environments. We’ve seen the lack of application level context have impacted user experiences, slowed down productivity, led to unexpected errors or misconfigurations in production.

On the other hand, modeling the deployment of a microservice application is a highly opinionated and fragmented process. Thus, many solutions tried to solve above problem so far essentially became restricted systems and barely extensible (regardless of whether they are built with Kubernetes or not). As the needs of your application grow, they are almost certain to outgrow the capabilities of such systems. Application teams complain they are too rigid and slow in response to feature requests or improvements. The platform team do want to help but the engineering effort is daunting: any simple change to such platform could easily become a marathon negotiation around the design of its abstraction.

## What is KubeVela?

KubeVela is a modern application platform that makes deploying and managing applications across today's hybrid, multi-cloud environments easier and faster without any restriction or limitation. This is achieved by doing the following:

**Application Centric** - KubeVela introduces declarative yet higher level API (known as [OAM](https://oam.dev/)) to model a full deployment of microservices across hybrid environments in consistent approach. No infrastructure level concerns, simply deploy.

**Natively Extensible** - KubeVela backend is implemented with [CUE](https://cuelang.org/). Whenever your needs grow, KubeVela's capabilities can naturally expand in a IaC-style approach. No restrictions, simply programming.

**Runtime Agnostic** - KubeVela relies on Kubernetes as control plane but it's adaptable to any runtime infrastructures. It can deploy and manage diverse workload types including container, cloud functions, databases, or even EC2 instances across hybrid environments.

## Architecture

The overall architecture of KubeVela is shown as below:

![alt](resources/arch.png)

### Control Plane

Control plane is where KubeVela itself lives in. As the project's name implies, KubeVela by design leverages Kubernetes as control plane. This is the key of how KubeVela guarantees full *automation* and strong *determinism* to application delivery at scale. Users will interact with KubeVela via the  application centric API to model the application deployment, and KubeVela will distribute it to target *runtime infrastructure* per policies and rules declared by users.

### Runtime Infrastructures

Runtime infrastructures are where the applications are actually running on. KubeVela allows you to model and deploy applications to any Kubernetes based infrastructure (either local, managed offerings, or IoT/Edge/On-Premise ones), or to public cloud platforms.

## Comparisons

### KubeVela vs. Platform-as-a-Service (PaaS) 

The typical examples are Heroku and Cloud Foundry. They provide full application deployment and management capabilities and aim to improve developer experience and efficiency. In this context, KubeVela shares the same goal.

Though the biggest difference lies in **flexibility**.

KubeVela is fully programmable. All capabilities in KubeVela are LEGO-sytle CUE modules and can be extended at any time when your needs grow. Comparing to this mechanism, traditional PaaS systems are highly restricted, i.e. they have to enforce constraints in the type of supported applications and capabilities, and as application needs grows, you always outgrow the capabilities of the PaaS system - this will never happen in KubeVela platform.

### KubeVela vs. Serverless  

Serverless platform such as AWS Lambda provides extraordinary user experience and agility to deploy serverless applications. However, those platforms impose even more constraints in extensibility. They are arguably "hard-coded" PaaS.

KubeVela can easily deploy both Kubernetes based serverless workloads such as Knative/OpenFaaS, or cloud based functions such as AWS Lambda.

### KubeVela vs. Platform agnostic developer tools

The typical example is Hashicorp's Waypoint. Waypoint is a developer facing tool which introduces a consistent workflow (i.e., build, deploy, release) to ship applications on top of different platforms.

KubeVela can be integrated with such tools seamlessly. In this case, developers would use the Waypoint workflow as the UI to deploy and release applications with KubeVela as the underlying deployment platform.

### KubeVela vs. Helm 

Helm is a package manager for Kubernetes that provides package, install, and upgrade a set of YAML files for Kubernetes as a unit. 

KubeVela as a modern deployment system can naturally deploy Helm charts. For example, you could use KubeVela to define an application that is composed by a WordPress chart and a AWS RDS Terraform module, orchestrate the components' topology, and then deploy them to multiple environments following certain strategy.

Furthermore, KubeVela also supports other encapsulation formats including Kustomize etc.

### KubeVela vs. Kubernetes

KubeVela is a Kubernetes add-on for building modern application deployment system. It leverages [Open Application Model](https://github.com/oam-dev/spec) and Kubernetes as control plane to resolve a hard problem - making shipping applications enjoyable.

## What's Next

Here are some recommended next steps:
- [Get started](./quick-start) with KubeVela.
- Learn KubeVela's [core concepts](./concepts).
- Learn how to [deploy an application](end-user/application) in detail and understand how it works.
- Join `#kubevela` channel in CNCF [Slack](https://cloud-native.slack.com) and/or [Gitter](https://gitter.im/oam-dev/community)

Welcome onboard and sail Vela!