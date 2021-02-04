# What is KubeVela?

This documentation explains "what KubeVela can do for you" in perspective of platform team.

## Overview

KubeVela provides several independent building blocks to help you create application platforms easily.

![alt](../../resources/kubevela-runtime.png)

### 1. Application Encapsulation

The encapsulation engine enables you to define an `Application` abstraction that encapsulates all the needed resources composed your app.

One typical use case is we want to encapsulate a Kubernetes `Deployment` and a `Service` into a module probably named *Web Service*, and let end users to instantiate this module by simply filling in the parameters (e.g. `image`, `replicas` and `ports`). For example, the [`web-service.ts` ](https://github.com/awslabs/cdk8s/blob/master/examples/typescript/web-service/web-service.ts) lib in cdk8s, the [`kube.cue`](https://github.com/cuelang/cue/blob/b8b489251a3f9ea318830788794c1b4a753031c0/doc/tutorial/kubernetes/quick/services/kube.cue#L70) lib in CUE, and this widely used [Deployment + Service](https://docs.bitnami.com/tutorials/create-your-first-helm-chart/) Helm chart. Of course, some teams with great frontend engineers will choose to build a GUI console to create such abstraction.

The `Application` abstraction supports all the scenarios above. From end user's view, an `Application` is assembled by components (workload specifications) and traits (operational behaviors), for example:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: Application
metadata:
  name: application-sample
spec:
  components:
    - name: foo
      type: worker # component type
      settings:
        image: "busybox"
        cmd:
        - sleep
        - "1000"
      traits:
        - name: scaler
          properties:
            replicas: 10
        - name: sidecar
          properties:
            name: "sidecar-test"
            image: "nginx"
    - name: bar
      type: aliyun-oss # component type
      bucket: "my-bucket"
```

In detail, every `component` and `trait` in above abstraction is defined by platform team via `Definition` objects. For example, [`WorkloadDefinition`](https://github.com/oam-dev/kubevela/tree/master/config/samples/application#workload-definition) and [`TraitDefinition`](https://github.com/oam-dev/kubevela/tree/master/config/samples/application#scaler-trait-definition). As the end user, they only need to assemble these modules into an application. Also, if end user has any new requirements, the platform team could customize the template in definitions by any time.

Besides this extensibility, there are several other benefits that the encapsulation engine can bring to you.

#### Unified Abstraction

KubeVela intends to support any possible module types as possible, for example `CUE`, `Terraform`, `Helm`, etc or just a plain Kubernetes CRD. This enables platform team to create unified abstraction that can model and deploy any kind of resource with ease, including cloud services, as long as they could be encapsulated by a supported module type. In the `application-sample` above, it defines a OSS bucket on Alibaba Cloud as a component which is powered by a Terraform module behind the scenes.

#### No Configuration Drift

Many of the existing modules today are defined by client side Infrastructure-as-Code (IaC) tools and even Kubernetes tool like Helm sits at client side as well. So in the nutshell, KubeVela encapsulation engine can just be implemented at client side which would be easier to be adopted.

But client side abstractions, though light-weighted, always lead to an issue called infrastructure/configuration drift, i.e. the generated component instances are not in line with the expected configuration. This could be caused by incomplete coverage, less-than-perfect processes or emergency changes.

In KubeVela, the encapsulation engine is intended to be implemented in a [Kubernetes Control Loop](https://kubernetes.io/docs/concepts/architecture/controller/). This is the key for KubeVela to eliminate the issue of configuration drifting but still keeps the simplicity and software delivery velocity enabled by IaC (and Helm) modules.

#### No "Juggling" Approach to Manage Kubernetes Objects

A typical use case is, as the platform team, we want to leverage `Istio` as the Service Mesh layer to control the traffic to certain `Deployment` instances. But this could be really painful today because we have to enforce end users to define and manage a set of Kubernetes resources in a "juggling" approach. For example, in a simple canary rollout case, the end users have to carefully manage a primary `Deployment`, a primary `Service`, a `root Service`, a canary `Deployment`, a canary `Service`, and have to probably rename the `Deployment` instance after canary promotion (this is actually unacceptable in production because renaming will lead to the app restart). What's worse, we have to expect the users properly set the labels and selectors on those objects carefully because they are the key to ensure proper accessibility of every app instance and the only revision mechanism our Istio controller could count on.

The issue above could be even painful if the workload instance is not `Deployment`, but `StatefulSet` or custom workload type. For example, normally it doesn't make sense to replicate a `StatefulSet` instance during rollout, this means the users have to maintain the name, revision, label, selector, app instances in a totally different approach from `Deployment`.

#### Standard Contract Behind The Abstraction

The encapsulation engine in KubeVela is designed to relieve such burden of managing versionized Kubernetes resources manually. In nutshell, all the needed Kubernetes resources for an app are now encapsulated in a single abstraction, and KubeVela will maintain the instance name, revisions, labels and selector by the battle tested reconcile loop automation, not by human hand. At the meantime, the existence of definition objects allow the platform team to customize the details of all above metadata behind the abstraction, even control the behavior of how to do revision.

Thus, all those metadata now become a standard contract that any day 2 operation controller such as Istio or rollout can rely on. This is the key to ensure our platform could provide user friendly experience but keep "transparent" to the operational behaviors.

### 2. Progressive Rollout

The deployment engine is responsible for progressive rollout of your app following given rollout strategy (e.g. canary, blue-green, etc).

> More information about this section is still work in progress.
