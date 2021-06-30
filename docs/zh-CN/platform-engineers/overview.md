---
title:  概述
---

本文档将解释什么是“Application”对象以及为什么需要它。

## 初衷

基于封装的抽象可能是最广泛使用的方法，可以使开发人员体验更轻松，并允许用户将整个应用程序资源作为一个单元交付。例如，今天许多工具将Kubernetes *Deployment* 和 *Service* 封装到一个 *Web Service* 模块中，然后通过简单地提供 *image=foo* 和 *ports=80* 等参数来实例化这个模块。这种模式可以在 cdk8s (例如 [`web-service.ts` ](https://github.com/awslabs/cdk8s/blob/master/examples/typescript/web-service/web-service.ts)), CUE (例如 [`kube.cue`](https://github.com/cuelang/cue/blob/b8b489251a3f9ea318830788794c1b4a753031c0/doc/tutorial/kubernetes/quick/services/kube.cue#L70))，以及许多广泛使用的 Helm charts 中找到(例如 [Web Service](https://docs.bitnami.com/tutorials/create-your-first-helm-chart/))。

尽管在定义抽象方面具有效率和可扩展性，但这两种 DSL 工具（例如 cdk8s 、CUE 和 Helm 模板）主要用作客户端工具，几乎不能用作平台级构建块。这使得平台构建者要么不得不创建受限/不可扩展的抽象，要么重新发明 DSL/templating 已经做得很好的轮子。

KubeVela 允许平台团队使用 DSL/templating 创建以开发人员为中心的抽象，但使用经过实战测试的 [Kubernetes 控制循环](https://kubernetes.io/docs/concepts/architecture/controller/) 来维护它们。

## Application

首先，KubeVela 引入了一个 `Application` CRD 作为其主要抽象，可以捕获完整的应用程序部署。 为了对最新的微服务进行建模，每个 Application 都由具有附加 trait（操作行为）的多个 components 组成。 例如：

```yaml
apiVersion: core.oam.dev/v1beta1
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

这个应用程序中 *component* 和 *trait* 规范的模式实际上是由另一组名为 *"definitions"* 的构建模块强制执行的，例如，`ComponentDefinition` 和 `TraitDefinition`。

`XxxDefinition` 资源旨在利用诸如 `CUE`、`Helm` 和 `Terraform modules` 等封装解决方案来模板化和参数化 Kubernetes 资源以及云服务。 这使用户能够通过简单地设置参数将模板化功能组装到 `Application` 中。 在上面的 `application-sample` 中，它模拟了一个 Kubernetes Deployment（component `foo`）来运行容器和一个阿里云 OSS 存储桶（component `bar`）。

这种抽象机制是 KubeVela 向最终用户提供 *PaaS-like* 体验（*即以应用程序为中心、更高级别的抽象、自助操作等*）的关键，其优势如下所示。

### 不再“杂耍”地管理 Kubernetes 对象

例如，作为平台团队，我们希望利用 Istio 作为服务网格层来控制某些 `Deployment` 实例的流量。但这在今天可能真的很难受，因为我们必须强制最终用户以有点“杂耍”的方式定义和管理一组 Kubernetes 资源。例如，在一个简单的金丝雀部署案例中，最终用户必须仔细管理一个主要的 *Deployment*、一个主要的 *Service*、一个 *root Service*、一个金丝雀 *Deployment*、一个金丝雀 *Service*，并且必须可能在金丝雀升级后重命名 *Deployment* 实例（这在生产中实际上是不可接受的，因为重命名会导致应用程序重新启动）。更糟糕的是，我们必须期望用户在这些对象上正确设置标签和选择器，因为它们是确保每个应用程序实例正确可访问的关键，也是我们 Istio 控制器可以依赖的唯一修订机制。

如果组件实例不是 *Deployment*，而是 *StatefulSet* 或自定义工作负载类型，则上述问题甚至可能会很痛苦。例如，通常在部署期间复制 *StatefulSet* 实例是没有意义的，这意味着用户必须以与 *Deployment* 完全不同的方法维护名称、修订、标签、选择器、应用程序实例。

#### 抽象背后的标准契约

KubeVela 旨在减轻手动管理版本化 Kubernetes 资源的负担。 简而言之，应用程序所需的所有 Kubernetes 资源现在都封装在一个抽象中，KubeVela 将通过经过实战测试的协调循环自动化而不是人工来维护实例名称、修订、标签和选择器。 同时，定义对象的存在让平台团队可以自定义抽象背后所有上述元数据的细节，甚至可以控制如何进行修订的行为。

因此，所有这些元数据现在都成为任何“day 2”操作控制器（例如 Istio 或 rollout）都可以依赖的标准合约。 这是确保我们的平台能够提供用户友好体验但对操作行为保持“透明”的关键。

### 无配置漂移

定义抽象的轻量级和灵活，当今任何现有的封装解决方案都可以在客户端工作，例如 DSL/IaC（基础设施即代码）工具和 Helm。 这种方式易于采用，对用户集群的入侵较少。

但是客户端抽象总是会导致一个称为*基础设施/配置漂移*的问题，即生成的组件实例与预期的配置不一致。 这可能是由不完整的覆盖范围、不完美的流程或紧急更改引起的。

因此，KubeVela 中的所有抽象都被设计为使用 [Kubernetes Control Loop](https://kubernetes.io/docs/concepts/architecture/controller/) 进行维护，并利用 Kubernetes 控制平面来消除配置漂移的问题，并且仍然保持现有封装解决方案（例如 DSL/IaC 和 templating）的灵活性和速度。
