---
title:  已知限制
---

## 限制

以下是使用 Helm 图表作为应用程序组件的一些已知限制。

### 工作负载类型指示器

遵循微服务的最佳实践，KubeVela 建议在一个 Helm 图表中只存在一种工作负载资源。 请将您的“超级”Helm 图表拆分为多个图表（即组件）。 本质上，KubeVela 依赖于组件定义中的`workload`来指示它需要注意的工作负载类型，例如：

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
...
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
```
```yaml
...
spec:
  workload:
    definition:
      apiVersion: apps.kruise.io/v1alpha1
      kind: Cloneset
```

请注意，如果多个工作负载类型打包在一个图表中，KubeVela 不会失败，问题在于进一步的操作行为，例如推出、修订和流量管理，它们只能对指定的工作负载类型生效。

### 始终使用完整的限定名称

工作负载的名称应使用 [完全限定的应用程序名称](https://github.com/helm/helm/blob/543364fba59b0c7c30e38ebe0f73680db895abb6/pkg/chartutil/create.go#L415) 进行模板化，并且请不要为`.Values.fullnameOverride`。作为最佳实践，Helm 还强烈建议通过 `$ helm create` 命令创建新图表，以便根据此最佳实践自动定义模板名称。

### 控制应用程序升级

对组件`properties` 所做的更改将触发 Helm 版本升级。此过程由 Flux v2 Helm 控制器处理，因此您可以定义基于 [Helm Release 文档](https://github.com/fluxcd/helm-controller/blob/main/docs/api/helmrelease.md#upgraderemediation) 和 [规范](https://toolkit.fluxcd.io/components/helm/helmreleases/#configuring-failure-remediation)的修复，以防在此升级过程中发生故障。

例如：
```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: webapp-chart
spec:
...
  schematic:
    helm:
      release:
        chart:
          spec:
            chart: "podinfo"
            version: "5.1.4"
        upgrade:
          remediation:
            retries: 3 
            remediationStrategy: rollback
      repository:
        url: "http://oam.dev/catalog/"

```

尽管目前存在一个问题，但很难获得有关 Helm 实时发布的有用信息，以了解升级失败时发生的情况。我们将增强可观察性，帮助用户在应用层面跟踪 Helm 发布的情况。

## 问题

已知问题将在后续版本中修复。

### 推出策略

目前，基于 Helm 的组件无法受益于 [应用程序级部署策略](https://github.com/oam-dev/kubevela/blob/master/design/vela-core/rollout-design.md#applicationdeployment-workflow)。如[本示例](./trait#update-an-applicatiion)所示，如果应用更新了，只能直接 rollout，没有 canary 或者 blue-green 方式。

### 更新特征属性也可能导致 Pod 重启

特性属性的更改可能会影响组件实例，属于此工作负载实例的 Pod 将重新启动。在基于 CUE 的组件中，这是可以避免的，因为 KubeVela 可以完全控制资源的渲染过程，尽管在基于 Helm 的组件中，它目前被推迟到 Flux v2 控制器。
