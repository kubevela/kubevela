---
title:  The Application Abstraction
---

This documentation will explain what is `Application` object and why you need it.

## Motivation

Encapsulation is probably the mostly widely used approach to enable easier developer experience and allow users to deliver the whole application resources as one unit. For example, many tools today encapsulate Kubernetes *Deployment* and *Service* into a *Web Service* module, and then instantiate this module by simply providing parameters such as *image=foo* and *ports=80*. This pattern can be found in cdk8s (e.g. [`web-service.ts` ](https://github.com/awslabs/cdk8s/blob/master/examples/typescript/web-service/web-service.ts)), CUE (e.g. [`kube.cue`](https://github.com/cuelang/cue/blob/b8b489251a3f9ea318830788794c1b4a753031c0/doc/tutorial/kubernetes/quick/services/kube.cue#L70)), and many widely used Helm charts (e.g. [Web Service](https://docs.bitnami.com/tutorials/create-your-first-helm-chart/)).

Despite the efficiency and extensibility in defining abstractions with encapsulation, both DSL tools (e.g. cdk8s , CUE and Helm templating) are mostly used as client side tools and can be barely used as a platform level building block. This leaves platform builders either have to create restricted/inextensible abstractions, or re-invent the wheels of what DSL/templating has already been doing great.

KubeVela allows platform teams to create developer-centric abstractions with DSL/templating but maintain them with the battle tested [Kubernetes Control Loop](https://kubernetes.io/docs/concepts/architecture/controller/). 

## Abstraction

First of all, KubeVela introduces an `Application` CRD as its main abstraction that could capture all needed resources to run the application, and exposes configurable parameters for end users. Every application is composed by multiple components, and each of them is defined by workload specification and traits (operational behaviors). For example:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: Application
metadata:
  name: application-sample
spec:
  components:
    - name: foo
      type: webservice
      properties:
        image: crccheck/hello-world
        port: 8000
      traits:
        - type: ingress
          properties:
            domain: testsvc.example.com
            http:
              "/": 8000
        - type: sidecar
          properties:
            name: "logging"
            image: "fluentd"
    - name: bar
      type: aliyun-oss # cloud service
      bucket: "my-bucket"
```

## Encapsulation

With `Application` provides an abstraction to deploy apps, each *component* and *trait* specification in this application is actually enforced by another set of building block objects named *"definitions"*, for example, `ComponentDefinition` and `TraitDefinition`.

Definitions are designed to leverage encapsulation technologies such as `CUE`, `Helm` and `Terraform modules` to template and parameterize Kubernetes resources as well as cloud services. This enables users to assemble templated capabilities into an `Application` by simply setting parameters. In the `application-sample` above, it models a Kubernetes Deployment (component `foo`) to run container and a Alibaba Cloud OSS bucket (component `bar`) alongside.

This encapsulation and abstraction mechanism is the key for KubeVela to provide *PaaS-like* experience (*i.e. app-centric, higher level abstractions, self-service operations etc*) to end users.

### No Configuration Drift

Many of the existing encapsulation solutions today work at client side, for example, DSL/IaC (Infrastructure as Code) tools and Helm. This approach is easy to be adopted and has less invasion in the user cluster.

But client side abstractions, though light-weighted, always lead to an issue called infrastructure/configuration drift, i.e. the generated component instances are not in line with the expected configuration. This could be caused by incomplete coverage, less-than-perfect processes or emergency changes.

Hence, the encapsulation engine of KubeVela is designed to be a [Kubernetes Control Loop](https://kubernetes.io/docs/concepts/architecture/controller/) and leverage Kubernetes control plane to eliminate the issue of configuration drifting, and still keeps the flexibly and velocity enabled by existing encapsulation solutions (e.g. DSL/IaC and templating).

### No "Juggling" Approach to Manage Kubernetes Objects

For example, as the platform team we want to leverage Istio as the Service Mesh layer to control the traffic to certain `Deployment` instances. But this could be really painful today because we have to enforce end users to define and manage a set of Kubernetes resources in a "juggling" approach. For example, in a simple canary rollout case, the end users have to carefully manage a primary *Deployment*, a primary *Service*, a *root Service*, a canary *Deployment*, a canary *Service*, and have to probably rename the *Deployment* instance after canary promotion (this is actually unacceptable in production because renaming will lead to the app restart). What's worse, we have to expect the users properly set the labels and selectors on those objects carefully because they are the key to ensure proper accessibility of every app instance and the only revision mechanism our Istio controller could count on.

The issue above could be even painful if the component instance is not *Deployment*, but *StatefulSet* or custom workload type. For example, normally it doesn't make sense to replicate a *StatefulSet* instance during rollout, this means the users have to maintain the name, revision, label, selector, app instances in a totally different approach from *Deployment*.

#### Standard Contract Behind The Abstraction

The encapsulation engine of KubeVela is designed to relieve such burden of managing versionized Kubernetes resources manually. In nutshell, all the needed Kubernetes resources for an app are now encapsulated in a single abstraction, and KubeVela will maintain the instance name, revisions, labels and selector by the battle tested reconcile loop automation, not by human hand. At the meantime, the existence of definition objects allow the platform team to customize the details of all above metadata behind the abstraction, even control the behavior of how to do revision.

Thus, all those metadata now become a standard contract that any day 2 operation controller such as Istio or rollout can rely on. This is the key to ensure our platform could provide user friendly experience but keep "transparent" to the operational behaviors.

