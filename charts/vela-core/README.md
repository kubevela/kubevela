<div style="text-align: center">
  <p align="center">
    <img src="https://raw.githubusercontent.com/oam-dev/kubevela.io/main/docs/resources/KubeVela-03.png">
    <br><br>
    <i>Make shipping applications more enjoyable.</i>
  </p>
</div>

![Build status](https://github.com/oam-dev/kubevela/workflows/E2E/badge.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/oam-dev/kubevela)](https://goreportcard.com/report/github.com/oam-dev/kubevela)
![Docker Pulls](https://img.shields.io/docker/pulls/oamdev/vela-core)
[![codecov](https://codecov.io/gh/oam-dev/kubevela/branch/master/graph/badge.svg)](https://codecov.io/gh/oam-dev/kubevela)
[![LICENSE](https://img.shields.io/github/license/oam-dev/kubevela.svg?style=flat-square)](/LICENSE)
[![Releases](https://img.shields.io/github/release/oam-dev/kubevela/all.svg?style=flat-square)](https://github.com/oam-dev/kubevela/releases)
[![TODOs](https://img.shields.io/endpoint?url=https://api.tickgit.com/badge?repo=github.com/oam-dev/kubevela)](https://www.tickgit.com/browse?repo=github.com/oam-dev/kubevela)
[![Twitter](https://img.shields.io/twitter/url?style=social&url=https%3A%2F%2Ftwitter.com%2Foam_dev)](https://twitter.com/oam_dev)
[![Artifact HUB](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/kubevela)](https://artifacthub.io/packages/search?repo=kubevela)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/4602/badge)](https://bestpractices.coreinfrastructure.org/projects/4602)

# KubeVela helm chart

KubeVela is a modern application platform that makes it easier and faster to deliver and manage applications across hybrid,
multi-cloud environments. At the mean time, it is highly extensible and programmable, which can adapt to your needs as they grow.

## TL;DR

```bash
helm repo add kubevela https://charts.kubevela.net/core
helm repo update
helm install --create-namespace -n vela-system kubevela kubevela/vela-core --wait
```

## Prerequisites

- Kubernetes >= v1.19 && < v1.22
  
## Parameters

### KubeVela core parameters

| Name                          | Description                                                                                   | Value     |
| ----------------------------- | --------------------------------------------------------------------------------------------- | --------- |
| `systemDefinitionNamespace`   | System definition namespace, if unspecified, will use built-in variable `.Release.Namespace`. | `nil`     |
| `applicationRevisionLimit`    | Application revision limit                                                                    | `10`      |
| `definitionRevisionLimit`     | Definition revision limit                                                                     | `20`      |
| `concurrentReconciles`        | concurrentReconciles is the concurrent reconcile number of the controller                     | `4`       |
| `controllerArgs.reSyncPeriod` | The period for resync the applications                                                        | `5m`      |
| `OAMSpecVer`                  | OAMSpecVer is the oam spec version controller want to setup                                   | `v0.3`    |
| `disableCaps`                 | Disable capability                                                                            | `rollout` |
| `enableFluxcdAddon`           | Whether to enable fluxcd addon                                                                | `false`   |
| `dependCheckWait`             | dependCheckWait is the time to wait for ApplicationConfiguration's dependent-resource ready   | `30s`     |


### KubeVela workflow parameters

| Name                                   | Description                                            | Value |
| -------------------------------------- | ------------------------------------------------------ | ----- |
| `workflow.backoff.maxTime.waitState`   | The max backoff time of workflow in a wait condition   | `60`  |
| `workflow.backoff.maxTime.failedState` | The max backoff time of workflow in a failed condition | `300` |
| `workflow.step.errorRetryTimes`        | The max retry times of a failed workflow step          | `10`  |


### KubeVela controller parameters

| Name                        | Description                          | Value              |
| --------------------------- | ------------------------------------ | ------------------ |
| `replicaCount`              | KubeVela controller replica count    | `1`                |
| `imageRegistry`             | Image registry                       | `""`               |
| `image.repository`          | Image repository                     | `oamdev/vela-core` |
| `image.tag`                 | Image tag                            | `latest`           |
| `image.pullPolicy`          | Image pull policy                    | `Always`           |
| `resources.limits.cpu`      | KubeVela controller's cpu limit      | `500m`             |
| `resources.limits.memory`   | KubeVela controller's memory limit   | `1Gi`              |
| `resources.requests.cpu`    | KubeVela controller's cpu request    | `50m`              |
| `resources.requests.memory` | KubeVela controller's memory request | `20Mi`             |
| `webhookService.type`       | KubeVela webhook service type        | `ClusterIP`        |
| `webhookService.port`       | KubeVela webhook service port        | `9443`             |
| `healthCheck.port`          | KubeVela health check port           | `9440`             |


### MultiCluster parameters

| Name                                                  | Description                      | Value                            |
| ----------------------------------------------------- | -------------------------------- | -------------------------------- |
| `multicluster.enabled`                                | Whether to enable multi-cluster  | `true`                           |
| `multicluster.clusterGateway.replicaCount`            | ClusterGateway replica count     | `1`                              |
| `multicluster.clusterGateway.port`                    | ClusterGateway port              | `9443`                           |
| `multicluster.clusterGateway.image.repository`        | ClusterGateway image repository  | `oamdev/cluster-gateway`         |
| `multicluster.clusterGateway.image.tag`               | ClusterGateway image tag         | `v1.1.7`                         |
| `multicluster.clusterGateway.image.pullPolicy`        | ClusterGateway image pull policy | `IfNotPresent`                   |
| `multicluster.clusterGateway.resources.limits.cpu`    | ClusterGateway cpu limit         | `100m`                           |
| `multicluster.clusterGateway.resources.limits.memory` | ClusterGateway memory limit      | `200Mi`                          |
| `multicluster.clusterGateway.secureTLS.enabled`       | Whether to enable secure TLS     | `true`                           |
| `multicluster.clusterGateway.secureTLS.certPath`      | Path to the certificate file     | `/etc/k8s-cluster-gateway-certs` |


### Test parameters

| Name                  | Description         | Value                |
| --------------------- | ------------------- | -------------------- |
| `test.app.repository` | Test app repository | `oamdev/hello-world` |
| `test.app.tag`        | Test app tag        | `v1`                 |
| `test.k8s.repository` | Test k8s repository | `oamdev/alpine-k8s`  |
| `test.k8s.tag`        | Test k8s tag        | `1.18.2`             |


### Common parameters

| Name                         | Description                                                                                                                | Value   |
| ---------------------------- | -------------------------------------------------------------------------------------------------------------------------- | ------- |
| `imagePullSecrets`           | Image pull secrets                                                                                                         | `[]`    |
| `nameOverride`               | Override name                                                                                                              | `""`    |
| `fullnameOverride`           | Fullname override                                                                                                          | `""`    |
| `serviceAccount.create`      | Specifies whether a service account should be created                                                                      | `true`  |
| `serviceAccount.annotations` | Annotations to add to the service account                                                                                  | `{}`    |
| `serviceAccount.name`        | The name of the service account to use. If not set and create is true, a name is generated using the fullname template     | `nil`   |
| `nodeSelector`               | Node selector                                                                                                              | `{}`    |
| `tolerations`                | Tolerations                                                                                                                | `[]`    |
| `affinity`                   | Affinity                                                                                                                   | `{}`    |
| `rbac.create`                | Specifies whether a RBAC role should be created                                                                            | `true`  |
| `logDebug`                   | Enable debug logs for development purpose                                                                                  | `false` |
| `logFilePath`                | If non-empty, write log files in this path                                                                                 | `""`    |
| `logFileMaxSize`             | Defines the maximum size a log file can grow to. Unit is megabytes. If the value is 0, the maximum file size is unlimited. | `1024`  |
| `kubeClient.qps`             | The qps for reconcile clients, default is 50                                                                               | `50`    |
| `kubeClient.brust`           | The burst for reconcile clients, default is 100                                                                            | `100`   |

## Uninstalling the Chart

To uninstall/delete the KubeVela helm release

```shell
$ helm uninstall -n vela-system kubevela
```

The command removes all the Kubernetes components associated with kubevela and deletes the release.

Notice: If you enable fluxcd addon  when install the chart by set `enableFluxcdAddon=true` .Uninstall wouldn't disable the fluxcd addon ,and it will be kept in the cluster.Please guarantee there is no application in cluster use this addon and disable it firstly before uninstall the helm chart. 





