# KubeVela: The Extensible App Platform Based on Open Application Model and Kubernetes

>7 Dec 2020 12:33pm, by Lei Zhang and Fei Guo

![image](https://tva1.sinaimg.cn/large/ad5fbf65gy1glgj5q8inej208g049aa6.jpg)

Last month at KubeCon+CloudNativeCon 2020, the [Open Application Model (OAM)](https://github.com/oam-dev/spec) community launched [KubeVela](https://github.com/oam-dev/kubevela/), an easy-to-use yet highly extensible application platform based on OAM and Kubernetes.

For developers, KubeVela is an easy-to-use tool that enables you to describe and ship applications to Kubernetes with minimal effort, yet for platform builders, KubeVela serves as a framework that empowers them to create developer-facing yet fully extensible platforms at ease.

The trend of cloud native technology is moving towards pursuing consistent application delivery across clouds and on-premises infrastructures using Kubernetes as the common abstraction layer. Kubernetes, although excellent in abstracting low-level infrastructure details, does introduce extra complexity to application developers, namely understanding the concepts of pods, port exposing, privilege escalation, resource claims, CRD, and so on. We’ve seen the nontrivial learning curve and the lack of developer-facing abstraction have impacted user experiences, slowed down productivity, led to unexpected errors or misconfigurations in production.

Abstracting Kubernetes to serve developers’ requirements is a highly opinionated process, and the resultant abstractions would only make sense had the decision-makers been the platform builders. Unfortunately, the platform builders today face the following dilemma: There is no tool or framework for them to easily extend the abstractions if any.

Thus, many platforms today introduce restricted abstractions and add-on mechanisms despite the extensibility of Kubernetes. This makes easily extending such platforms for developers’ requirements or to wider scenarios almost impossible.

In the end, developers complain those platforms are too rigid and slow in response to feature requests or improvements. The platform builders do want to help but the engineering effort is daunting: any simple API change in the platform could easily become a marathon negotiation around the opinionated abstraction design.

## Introducing KubeVela

With KubeVela, we aim to solve these two challenges in an approach that separates concerns of developers and platform builders.

For developers, KubeVela is an easy-to-use yet extensible tool that enables you to describe and deploy microservices applications with minimal effort. And instead of managing a handful of Kubernetes YAML files, a simple docker-compose style `appfile` is all you need.

### A Sample Appfile

In this example, we will create a vela.yaml along with your app. This file describes how to build the image, how to deploy the image to Kubernetes, how to access the application and how the system would scale it automatically.

```yaml
name: testapp

services:
    express-server:
      image: oamdev/testapp:v1
      build:
        docker:
          file: Dockerfile
          contrxt: .
      cmd: ["node", "server.js"]
      port: 8080
      cpu: "0.01"

      route:
        domain: example.com
        rules:
          - path: /testapp
            rewriteTarget: /

      autoscale:
        min: 1
        max: 4
        cpuPercent: 5
```

Just do: `$ vela up`, your app will then be alive on  https://example.com/testapp.

### Behind the Appfile

The `appfile` in KubeVela does not have a fixed schema specification, instead, what you can define in this file is determined by what kind of workload types and traits are available in your platform. These two concepts are core concepts from OAM, in detail:

- [Workload type](https://kubevela.io/%23/en/concepts?id=workload-type-amp-trait), which declares the characteristics that runtime infrastructure should take into account in application deployment. In the sample above, it defines a “Web Service” workload named `express-server` as part of your application.
- [Trait](https://kubevela.io/%23/en/concepts?id=workload-type-amp-trait), which represents the operation configurations that are attached to an instance of workload type. Traits augment a workload type instance with operational features. In the sample above, it defines a route trait to access the application and an autoscale trait for the CPU based horizontal automatic scaling policy.

Whenever a new workload type or trait is added, it would become immediately available to be declared in the `appfile`. Let’s say, a new trait named metrics is added, developers could check the schema of this trait by simply `$ vela show metrics` and define it in the previous sample `appfile`:

```yaml
name: testapp

services:
    express-server:
      type: webservice
      image: oamdev/testapp:v1
      build:
        docker:
          file: Dockerfile
          contrxt: .
      cmd: ["node", "server.js"]
      port: 8080
      cpu: "0.01"

      route:
        domain: example.com
        rules:
          - path: /testapp
            rewriteTarget: /

      autoscale:
        min: 1
        max: 4
        cpuPercent: 5

      metrices:
        port: 8080
        path: "/metrics"
        scheme: "http"
        enabled: true
```

## Vela Up

The `vela up` command deploys the application defined in `appfile` to Kubernetes. After deployment, you can use `vela status` to check how to access your application following the `route` trait declared in `appfile`.

![](https://tvax2.sinaimg.cn/large/ad5fbf65gy1glf9pyhr42j20la0kiafn.jpg)

Apps deployed with KubeVela will receive a URL (and versioned pre-release URLs) with valid TLS certificate automatically generated via [cert-manager](https://cert-manager.io/docs/). KubeVela also provides a set of commands (i.e. `vela logs, vela exec`) to best support your application management without becoming a Kubernetes expert. [Learn more about vela up and appfile](https://kubevela.io/%23/en/developers/learn-appfile).

## KubeVela for Platform Builders

The above experience cannot be achieved without KubeVela’s innovative offerings to the platform builders as an extensible platform engine. These features are the hidden gems that make KubeVela unique. In details, KubeVela relieves the pains of building developer facing platforms on Kubernetes by doing the following:

- **Application Centric**. Behind the appfile, KubeVela enforces “application” as its main API and all KubeVela’s capabilities serve the applications’ requirements only. This is how KubeVela brings application-centric context to the platform by default and changes building such platforms into working around application architecture.
- **Extending Natively**. As mentioned in the developer section, an application described by appfile is composed of various pluggable workload types and operation features (i.e. traits). Capabilities from Kubernetes ecosystem can be added to KubeVela as new workload types or traits through Kubernetes CRD registry mechanism at any time.
- **Simple yet Extensible User Interface**. Behind the `appfile`, KubeVela uses [CUELang](https://github.com/cuelang/cue) as the “last mile” abstraction engine between user-facing schema and the control plane objects. KubeVela provides a set of built-in abstractions to start with and the platform builders are free to modify them at any time. Capability adding/updating or abstraction changes will all take effect at runtime, neither recompilation nor redeployment of KubeVela is required.

Under the hood, KubeVela core is built on top of Crossplane OAM Kubernetes Runtime with KEDA, Flagger, Prometheus, etc as dependencies, yet its feature pool is “unlimited” and can be extended at any time.

![](https://tva2.sinaimg.cn/large/ad5fbf65gy1glf9sktkdxj20q00dsacl.jpg)

With KubeVela, platform builders now have the tooling support to design and ship any new capabilities with abstractions to end-users with high confidence and low turnaround time. And for a developer, you only need to learn these abstractions, describe the app with them in a single file, and then ship it.

## Not Another PaaS System

Most typical Platform-as-a-Service (PaaS) systems also provide full application management capabilities and aim to improve developer experience and efficiency. In this context, KubeVela shares the same goal.

Though unlike most typical PaaS systems which are either inextensible or create their own addon systems maintained by their own communities. KubeVela is designed to fully leverage the Kubernetes ecosystems as its capability pool. Hence, there’s no additional addon system introduced in this project. For platform builders, a new capability can be installed in KubeVela at any time by simply registering its API resource to OAM and providing a CUE template. We hope and expect that with the help of the open source community, the number of the KubeVela’s capabilities will grow dramatically over time. [Learn more about using community capabilities by $vela cap](https://kubevela.io/%23/en/developers/cap-center).

So in a nutshell, KubeVela is a Kubernetes plugin for building application-centric abstractions. It leverages the native Kubernetes extensibility and capabilities to resolve a hard problem – making application management enjoyable on Kubernetes.

## Learn More

KubeVela is incubated by the OAM community as the successor of [Rudr](https://github.com/oam-dev/rudr) project, while rather than being a reference implementation, KubeVela intends to be an end-to-end implementation that could be used in wider scenarios. The design of KubeVela’s appfile is also part of the experimental attempt in OAM specification to bring a simplified user experience to developers.

To learn more about KubeVela, please visit KubeVela’s [documentation site](https://kubevela.io/). The following content are also good next steps:

- Try out KubeVela following the [step-by-step tutorial](https://kubevela.io/%23/en/quick-start) in its Quick Start page.
- Give us feedback! KubeVela is still in its early stage and we are happy to ask the community for feedback via OAM [Gitter](https://gitter.im/oam-dev/community) or [Slack channel](https://cloud-native.slack.com/archives/C01BLQ3HTJA).
- [Extend KubeVela](https://kubevela.io/%23/en/platform-engineers/trait) to build your own platforms. If you have an idea for a new workload type, trait or try to build something more complex like a database or AI PaaS with KubeVela, post your idea as a GitHub Issue or propose it to the OAM community, we are eager to know.
- [Contribute to KubeVela](https://github.com/oam-dev/kubevela/blob/master/CONTRIBUTING.md). KubeVela is initialized by the open source community with [bootstrap contributors](https://github.com/oam-dev/kubevela/blob/bbb2c527d96d3e1a0694e2f49b3d1d1168e72c53/OWNERS_ALIASES%23L35) from 8+ different organizations. We intend to donate this project to a neutral foundation as soon as it gets stable.