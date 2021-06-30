---
title:  更多用法
---

KubeVela 中的组件旨在由用户带来。

## 1. 从能力中心获取

KubeVela 允许您探索由平台团队维护的功能。
kubectl vela 插件中有两个命令：`comp` 和 `trait`。

如果您尚未安装 kubectl vela 插件：请参阅 [这里](../../developers/references/kubectl-plugin#install-kubectl-vela-plugin)。

### 1. 列表

例如，让我们尝试列出注册表中的所有可用组件：

```shell
$ kubectl vela comp --discover
Showing components from registry: https://registry.kubevela.net
NAME              	REGITSRY	DEFINITION                 	
cloneset          	default	    clonesets.apps.kruise.io
kruise-statefulset	default	    statefulsets.apps.kruise.io
openfaas          	default	    functions.openfaas.com
````
请注意，`--discover` 标志表示显示不在集群中的所有组件。

### 2.安装
然后你可以安装一个组件，如：

```shell
$ kubectl vela comp get cloneset
Installing component capability cloneset
Successfully install trait: cloneset                                                                                                 
```

### 3.验证

```shell
$ kubectl get componentdefinition  -n vela-system
NAME         WORKLOAD-KIND   DESCRIPTION
cloneset     CloneSet        Describes long-running, scalable, containerized services that have a stable network endpoint to receive external network traffic from customers. It was implemented by OpenKruise Cloneset.
...(other componentDefinition)

```

默认情况下，这两个命令将从 KubeVela 维护的 [repo](https://registry.kubevela.net) 中检索功能。

## 2. 自己设计
查看以下文档，了解如何以各种方法将您自己的组件引入系统。

- [Helm](../../platform-engineers/helm/component) - Helm chart 是组件的一种自然形式，请注意，在这种情况下，您需要有一个有效的 Helm 存储库（例如 GitHub 存储库或 Helm 中心）来托管 chart。
- [CUE](../../platform-engineers/cue/component) - CUE 是封装组件的强大方法，它不需要任何存储库。
- [Simple Template](../../platform-engineers/kube/component) - 不是 Helm 或 CUE 专家？ 还提供了一种简单的模板方法来将任何 Kubernetes API 资源定义为一个组件。 请注意，在这种情况下仅支持键值样式参数。
- [Cloud Services](../../platform-engineers/cloud-services) - KubeVela 允许您将云服务声明为应用程序的一部分，并在一致的 API 中提供它们。
