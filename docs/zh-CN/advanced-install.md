---
title:  安装（进阶）
---

## 带着证书管理器安装 KubeVela

KubeVela 可以使用证书管理器为你的应用生成证书，但是你需要提前安装好证书管理器。

```shell script
helm repo add jetstack https://charts.jetstack.io
helm repo update
helm install cert-manager jetstack/cert-manager --namespace cert-manager --version v1.2.0 --create-namespace --set installCRDs=true
```

安装 KubeVela 同时启用证书管理器：

```shell script
helm install --create-namespace -n vela-system --set admissionWebhooks.certManager.enabled=true kubevela kubevela/vela-core
```

## 安装预发布版

在使用 `helm search` 命令时，添加标记参数 `--devel` 即可搜索出预发布版。预发布版的版本号格式为 `<next_version>-rc-master`，例如 `0.4.0-rc-master`，代表的是一个基于 `master` 分支构建的发布候选版。

```shell script
helm search repo kubevela/vela-core -l --devel
```
```console
    NAME                      CHART VERSION         APP VERSION           DESCRIPTION
    kubevela/vela-core        0.4.0-rc-master         0.4.0-rc-master         A Helm chart for KubeVela core
    kubevela/vela-core        0.3.2                   0.3.2                   A Helm chart for KubeVela core
    kubevela/vela-core        0.3.1                 0.3.1                 A Helm chart for KubeVela core
```

然后尝试跟着以下的命令安装一个预发布版。

```shell script
helm install --create-namespace -n vela-system kubevela kubevela/vela-core --version <next_version>-rc-master
```
```console
NAME: kubevela
LAST DEPLOYED: Thu Apr  1 19:41:30 2021
NAMESPACE: vela-system
STATUS: deployed
REVISION: 1
NOTES:
Welcome to use the KubeVela! Enjoy your shipping application journey!
```

## 升级

### 第一步 更新 Helm 仓库

通过以下命令获取 KubeVela 最新发布的 chart：

```shell
helm repo update
helm search repo kubevela/vela-core -l
```

### 第二步 升级 KubeVela 的 CRDs

```shell
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/charts/vela-core/crds/core.oam.dev_componentdefinitions.yaml
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/charts/vela-core/crds/core.oam.dev_workloaddefinitions.yaml
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/charts/vela-core/crds/core.oam.dev_traitdefinitions.yaml
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/charts/vela-core/crds/core.oam.dev_applications.yaml
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/charts/vela-core/crds/core.oam.dev_approllouts.yaml
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/charts/vela-core/crds/core.oam.dev_applicationrevisions.yaml
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/charts/vela-core/crds/core.oam.dev_scopedefinitions.yaml
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/charts/vela-core/crds/core.oam.dev_appdeployments.yaml
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/charts/vela-core/crds/core.oam.dev_applicationcontexts.yaml
```

> 提示：如果看到诸如 `* is invalid: spec.scope: Invalid value: "Namespaced": filed is immutable` 之类的错误，请删除出错的 CRD 后再重新安装。

```shell
 kubectl delete crd \
  scopedefinitions.core.oam.dev \
  traitdefinitions.core.oam.dev \
  workloaddefinitions.core.oam.dev
```

### 第三步 升级 KubeVela Helm chart

```shell
helm upgrade --install --create-namespace --namespace vela-system  kubevela kubevela/vela-core --version <the_new_version>
```

## 卸载

运行命令:

```shell script
helm uninstall -n vela-system kubevela
rm -r ~/.vela
```

命令会卸载 KubeVela 服务和相关的依赖组件，同时会清理本地 CLI 的缓存  
然后清理 CRDs（默认情况下，helm 不会移除 CRDs）

```shell script
 kubectl delete crd \
  appdeployments.core.oam.dev \
  applicationconfigurations.core.oam.dev \
  applicationcontexts.core.oam.dev \
  applicationrevisions.core.oam.dev \
  applications.core.oam.dev \
  approllouts.core.oam.dev \
  componentdefinitions.core.oam.dev \
  components.core.oam.dev \
  containerizedworkloads.core.oam.dev \
  healthscopes.core.oam.dev \
  manualscalertraits.core.oam.dev \
  podspecworkloads.standard.oam.dev \
  scopedefinitions.core.oam.dev \
  traitdefinitions.core.oam.dev \
  workloaddefinitions.core.oam.dev
```