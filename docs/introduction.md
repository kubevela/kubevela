# Introduction to KubeVela

![alt](../resources/KubeVela-01.png)

Welcome to KubeVela! This documentation covers why built KubeVela, what KubeVela is, and comparison of KubeVela vs. others.

## Why KubeVela?

The modern patterns of cloud-native application management has been merited with consistency across clouds and infrastructure by abstracting away the low level details with Kubernetes. However, this also introduced another layer of complexity: pods, ingresses, claims, service meshes, and so on. We've seen the lack of Kubernetes knowledge and its high learning curve have impacted user experience, slowed down the software shipping, led to user misconfiguration and production issues. Developers start questioning the value of cloud-native revolution, "why bother me with those YAML files?".

Another challenge we've seen is in solving this problem. There are many successful platforms (including PaaS and Serverless) in the community that brought great developer experience to cloud-native world. But for platform teams in many organizations, this poses a new challenge in how to adapt or extend these platforms to meet their own needs in their own scenarios. Essentially, platforms aiming at fixing developer facing issues tend to be highly opinionated in terms of user interfaces, assumptions and implementations. This makes extending such platforms leveraging wider community (e.g. Kubernetes ecosystem) almost impossible, and many of these platforms essentially become close systems.

KubeVela intends to make shipping applications more enjoyable for developers, and at the same time, provides platform engineers with full extensibility to build their own platforms leveraging the Kubernetes community.

## What is KubeVela?

*For developers, KubeVela is an easy-to-use tool that enables developers to describe and ship their applications with simple commands.*

In this context, KubeVela provides ease of use on top of Kubernetes to enable developers ship their softwares with efficiency and confidence to any cluster. No more API objects, no more container patterns. This workflow could be easily automated by CI/CD pipelines.

*For platform engineers, KubeVela is an extensible engine where platform builders can create something more complex on top of, in an easy and Kubernetes native approach.*

In this context, KubeVela aims at providing full extensibility to enable platform engineers build more complete platforms to serve their own scenarios. KubeVela allows platform engineers to bring in capabilities from the Kubernetes ecosystem in an easy and native approach, and the newly added capability would become immediately accessible in developers' workflow.

## KubeVela vs. Others

### Heroku, Cloud Foundry, PaaS, etc.

Platform-as-a-Service (PaaS) such as Heroku and Cloud Foundry typically provides full application management capabilities and enable developers to ship applications with great user experience and efficiency.

KubeVela has the similar goal in terms of helping developers to ship applications easily and quickly, though KubeVela's built-in feature set is much smaller than many existing PaaS offerings, it's more like a "micro-PaaS".

To deliver the best developer experience without losing control of the platform, most PaaS systems introduce constrains on the type of application they support, the operational capabilities they provide, and the way of how to extend the platforms. As result, PaaS systems are either inextensible, or tend to create its own addon system and community.

KubeVela is built with the assumption that every capability is an addon which could be provided by Kubernetes itself or a CRD controller. Thus, KubeVela doesn't have an addon system, instead, platform administrators are free to install a new capability to KubeVela at any time, by simply "register" its API resource in KubeVela with single command.

The extensibility based on Kubernetes ecosystem is the main difference between KubeVela vs. most PaaS systems. 

### Serverless platforms, etc.

Serverless platforms such as FaaS has even better experience and agility to deploy and operate the applications, while the cost is in this case, platform teams have much less freedom on the extensibility side: serverless platforms are typically much more opinionated than transitional PaaS.

KubeVela doesn't support serverless workload as built-in feature. However, for any serverless platform based on Kubernetes (e.g. Knative, OpenFaaS, etc.), they should be easy to be integrated into KubeVela as a new application type.

Even for serverless cloud offerings like AWS Lambda, it can also be integrated into KubeVela with help of Crossplane or AWS Controller for Kubernetes (ACK).

### Waypoint, etc.

Waypoint is a developer facing tool which introduced a consistent workflow (i.e. build, deploy, release) to ship an application. Waypoint is platform agnostic.

KubeVela is an application platform based on Kubernetes and doesn't define specific developer workflow. However, KubeVela's Appfile could indeed bring similar experience of `waypoint up` to Kubernetes in many cases. 

KubeVela can be integrated with Waypoint like any other platforms, and it should be easy considering the application-centric API KubeVela speaks. In this case, developers will use Waypoint workflow instead of the KubeVela's CLI.

This comparison also applies to many other platform agnostic developer tools such as `The Serverless Framework`, etc.


### Helm, etc.

Helm is a package manager for Kubernetes that provides package, install, and upgrade a set of YAML files for Kubernetes as a unit. 

KubeVela is not a package manager and can't be used to manage or deploy Kubernetes YAML files by default. KubeVela provides a client side tool named Appfile to describe how to build and deploy application in a single file but it's not Kubernetes YAML format. However, KubeVela server side component indeed exposes a main API resource named "Application Configuration" (i.e. the object behind KubeVela Appfile and CLI), it could be packaged and distributed by Helm at ease.

In implementation side, KubeVela heavily relies on Helm to package and manage the third-party plug-ins such as `Prometheus`, etc. 

### Kubernetes

KubeVela is a Kubernetes extension, it's complementary to Kubernetes.

In detail, KubeVela introduced several developer-centric abstractions to Kubernetes such as `Application`, etc and leverages capabilities of Kubernetes as underlying implementation. KubeVela also allows platform engineers to bring in any other capabilities from Kubernetes ecosystem with minimal effort. In nutshell, KubeVela is an highly extensible application management plugin for Kubernetes.