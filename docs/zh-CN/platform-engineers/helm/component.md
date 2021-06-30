---
title:  怎么用 helm
---

在本节中，将介绍如何通过 `ComponentDefinition` 将 Helm charts 声明为应用程序组件。

> 在阅读本部分之前，请确保您已经了解了[定义和模板概念](../definition-and-templates)。

## 先决条件

* [fluxcd/flux2](../../install#3-optional-install-flux2)，请确保您已经在[安装指南](/docs/install)中安装了 flux2。

## 声明 `ComponentDefinition`

这是一个关于如何使用 Helm 作为 schematic 模块的示例 `ComponentDefinition`。

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: webapp-chart
  annotations:
    definition.oam.dev/description: helm chart for webapp
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  schematic:
    helm:
      release:
        chart:
          spec:
            chart: "podinfo"
            version: "5.1.4"
      repository:
        url: "http://oam.dev/catalog/"
```

详细：
- 需要`.spec.workload` 来指示这个基于 Helm 的组件的工作负载类型。 如果您将多个工作负载打包在一个 chart 中，请同时检查 [已知限制](./known-issues#=workload-type-indicator)。
- `.spec.schematic.helm` 包含 Helm `release` 和利用 `fluxcd/flux2` 的 `repository` 的信息。
   - 即`release`的pec与[`HelmReleaseSpec`](https://github.com/fluxcd/helm-controller/blob/main/docs/api/helmrelease.md) 对齐，`repository`的 spec 和[`HelmRepositorySpec`](https://github.com/fluxcd/source-controller/blob/main/docs/api/source.md#source.toolkit.fluxcd.io/v1beta1.HelmRepository)对齐。

## 声明一个`Application`

这是一个示例 `Application`。

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: myapp
  namespace: default
spec:
  components:
    - name: demo-podinfo 
      type: webapp-chart 
      properties: 
        image:
          tag: "5.1.2"
```

组件 `properties` 正是 Helm Chart 的 [overlay values](https://github.com/captainroy-hy/podinfo/blob/master/charts/podinfo/values.yaml)。

部署应用程序，几分钟后（获取 Helm Chart 可能需要一些时间），您可以检查 Helm 版本是否已安装。
```shell
$ helm ls -A
myapp-demo-podinfo  default   1   2021-03-05 02:02:18.692317102 +0000 UTC deployed  podinfo-5.1.4     5.1.4
```
检查 Chart 中定义的工作负载是否已成功创建。
```shell
$ kubectl get deploy
NAME                     READY   UP-TO-DATE   AVAILABLE   AGE
myapp-demo-podinfo   1/1     1            1           66m
```

检查应用程序的 `properties` 中的值（`image.tag = 5.1.2`）是否已分配给 Chart 。
```shell
$ kubectl get deployment myapp-demo-podinfo -o json | jq '.spec.template.spec.containers[0].image'
"ghcr.io/stefanprodan/podinfo:5.1.2"
```


### 从基于 Helm 的组件生成表单

KubeVela 会根据 Helm Chart 中的 [`values.schema.json`](https://helm.sh/docs/topics/charts/#schema-files) 自动生成 OpenAPI v3 JSON schema，并将其存储在一个 ` ConfigMap` 在与定义对象相同的 `namespace` 中。 此外，如果 Chart 作者未提供 `values.schema.json`，KubeVela 将根据其 `values.yaml` 文件自动生成 OpenAPI v3 JSON 模式。

请查看 [Generate Forms from Definitions](/docs/platform-engineers/openapi-v3-json-schema) 指南，了解有关使用此架构呈现 GUI 表单的更多详细信息。
