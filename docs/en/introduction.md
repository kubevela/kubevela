---
title:  Introduction
slug: / 

---

![alt](resources/KubeVela-01.png)

## Motivation

The trend of cloud-native technology is moving towards pursuing consistent application delivery across clouds and on-premises infrastructures using Kubernetes as the common layer. Kubernetes, although excellent in abstracting low-level infrastructure details, does not introduce abstractions to model software deployment on top of hybrid environments. We’ve seen the lack of such application level context have impacted user experiences, slowed down productivity, led to unexpected errors or misconfigurations in production.

On the other hand, modeling a deployment of a modern microservice application is a highly opinionated and fragmented process today. Thus, most solutions aim to solve above problem (although built with Kubernetes) are essentially restricted systems and barely extensible. As the needs of your application grow, they are almost certain to outgrow the capabilities of such systems. Application teams complain they are too rigid and slow in response to feature requests or improvements. The platform team do want to help but the engineering effort is daunting: any simple API change in the platform could easily become a marathon negotiation around the opinionated abstraction design.

## What is KubeVela?

KubeVela is a modern application platform that aims to make deploying and managing applications across hybrid, multi-cloud environments easier and faster by doing the following:

**Application Centric** - KubeVela introduces consistent yet application centric API to capture a full deployment of microservices on top of hybrid environments. No infrastructure level concern, simply deploy.

**Natively Extensible** - KubeVela uses CUE to glue capabilities provided by runtime infrastructure and expose them to users via self-service API. When users' needs grow, these API can naturally expand in programmable approach.

**Runtime Agnostic** - KubeVela is built with Kubernetes as control plane but adaptable to any runtime as data-plane. It can deploy (and manage) diverse workload types such as container, cloud functions, databases, or even EC2 instances across hybrid environments.

## Architecture

The overall architecture of KubeVela is shown as below:

![alt](resources/arch.png)

### Control Plane

Control plane is where KubeVela itself lives in. As the project's name implies, KubeVela by design leverages Kubernetes as control plane. This is the key of how KubeVela brings full *automation* and strong *determinism* to application delivery at scale. Users will interact with KubeVela via the  application centric API to model the application deployment, and KubeVela will distribute it to target *runtime infrastructure* per policies and rules declared by users.

### Runtime Infrastructures

Runtime infrastructures are where the applications are actually running on. KubeVela allows you to model and deploy applications to any Kubernetes based infrastructure (either local, managed offerings, or IoT/Edge/On-Premise ones), or to public cloud platforms.

## Comparisons

### KubeVela vs. Platform-as-a-Service (PaaS) 

The typical examples are Heroku and Cloud Foundry. They provide full application deployment and management capabilities and aim to improve developer experience and efficiency. In this context, KubeVela shares the same goal.

Though the biggest difference lies in **flexibility**.

KubeVela enables you to serve end users with programmable building blocks (based on [CUE](https://cuelang.org/)) which are fully flexible and can be extended at any time. Comparing to this mechanism, traditional PaaS systems are highly restricted, i.e. they have to enforce constraints in the type of supported applications and capabilities, and as application needs grows, you always outgrow the capabilities of the PaaS system - this will never happen in KubeVela platform.

So think of KubeVela as a Heroku but it is fully extensible when your needs grow.

### KubeVela vs. Serverless  

Serverless platform such as AWS Lambda provides extraordinary user experience and agility to deploy serverless applications. However, those platforms impose even more constraints in extensibility. They are arguably "hard-coded" PaaS.

KubeVela can easily deploy both Kubernetes based serverless workloads such as Knative/OpenFaaS, or cloud functions such as AWS Lambda. Simply register what you want to deploy as a "component".

### KubeVela vs. Platform agnostic developer tools

The typical example is Hashicorp's Waypoint. Waypoint is a developer facing tool which introduces a consistent workflow (i.e., build, deploy, release) to ship applications on top of different platforms.

KubeVela can be integrated with such tools seamlessly. In this case, developers would use the Waypoint workflow as the UI to deploy and manage applications across hybrid environments with KubeVela's abstractions (e.g. applications, components, traits etc).

### KubeVela vs. Helm 

Helm is a package manager for Kubernetes that provides package, install, and upgrade a set of YAML files for Kubernetes as a unit. 

KubeVela as a modern deployment system can naturally deploys Helm charts across hybrid environments. For example, you could easily use KubeVela to declare and deploy an application which is composed by a WordPress Helm chart and a AWS RDS instance defined by Terraform, or distribute the Helm chart to multiple clusters.

KubeVela also leverages Helm to manage the capability addons in runtime clusters.

### KubeVela vs. Kubernetes

KubeVela is a Kubernetes add-on for building developer-centric deployment system. It leverages [Open Application Model](https://github.com/oam-dev/spec) and the native Kubernetes extensibility to resolve a hard problem - making shipping applications enjoyable on Kubernetes.


## What's Next

Here are some recommended next steps:
- [Get started](./quick-start) with KubeVela.
- Learn KubeVela's [core concepts](./concepts).
- Learn how to [deploy an application](end-user/application) in detail and understand how it works.
- Join `#kubevela` channel in CNCF [Slack](https://cloud-native.slack.com) and/or [Gitter](https://gitter.im/oam-dev/community)

Welcome onboard and sail Vela!