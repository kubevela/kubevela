<div style="text-align: center">
  <p align="center">
    <img src="https://raw.githubusercontent.com/kubevela/kubevela.io/main/docs/resources/KubeVela-03.png">
    <br><br>
    <i>Make shipping applications more enjoyable.</i>
  </p>
</div>

![Build status](https://github.com/kubevela/kubevela/workflows/E2E/badge.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/kubevela/kubevela)](https://goreportcard.com/report/github.com/kubevela/kubevela)
![Docker Pulls](https://img.shields.io/docker/pulls/oamdev/vela-core)
[![codecov](https://codecov.io/gh/kubevela/kubevela/branch/master/graph/badge.svg)](https://codecov.io/gh/kubevela/kubevela)
[![LICENSE](https://img.shields.io/github/license/kubevela/kubevela.svg?style=flat-square)](/LICENSE)
[![Releases](https://img.shields.io/github/release/kubevela/kubevela/all.svg?style=flat-square)](https://github.com/kubevela/kubevela/releases)
[![TODOs](https://img.shields.io/endpoint?url=https://api.tickgit.com/badge?repo=github.com/kubevela/kubevela)](https://www.tickgit.com/browse?repo=github.com/oam-dev/kubevela)
[![Twitter](https://img.shields.io/twitter/url?style=social&url=https%3A%2F%2Ftwitter.com%2Foam_dev)](https://twitter.com/oam_dev)
[![Artifact HUB](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/kubevela)](https://artifacthub.io/packages/search?repo=kubevela)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/4602/badge)](https://bestpractices.coreinfrastructure.org/projects/4602)

# KubeVela Minimal helm chart

KubeVela is a modern application platform that makes deploying and managing applications across today's hybrid, multi-cloud environments easier and faster.

KubeVela Minimal is the minimal version of KubeVela, it only contains the minimal components to run KubeVela and do not support features like multi clusters and env binding. For more complete experience, please install the [full version of KubeVela]((https://kubevela.io/docs/install)).

KubeVela Minimal contains the following CRDs:

CRD Name | Usage
------------ | -------------
applications.core.oam.dev  | KubeVela Core Application Object
applicationrevisions.core.oam.dev | The revision CRD of Application
componentdefinitions.core.oam.dev | ComponentDefinition Object
traitdefinitions.core.oam.dev  | TraitDefinition Object
workflowstepdefinitions.core.oam.dev | Workflowstep Object
scopedefinitions.core.oam.dev | ScopeDefinition Object
policydefinitions.core.oam.dev | PolicyDefinition Object
 workloaddefinitions.core.oam.dev  | WorkloadDefinition Object
definitionrevisions.core.oam.dev | The revision CRD for all definition objects
rollouts.standard.oam.dev | Rollout feature trait
resourcetrackers.core.oam.dev | Garbage Collection feature
healthscopes.core.oam.dev | Health Check feature

## TL;DR

```bash
helm repo add kubevela https://charts.kubevela.net/core
helm repo update
helm install --create-namespace -n vela-system kubevela kubevela/vela-minimal --wait
```

## Prerequisites

- Kubernetes >= v1.19 && < v1.22
  
## Parameters

### KubeVela core parameters

| Name                          | Description                                                                                   | Value                |
| ----------------------------- | --------------------------------------------------------------------------------------------- | -------------------- |
| `systemDefinitionNamespace`   | System definition namespace, if unspecified, will use built-in variable `.Release.Namespace`. | `nil`                |
| `applicationRevisionLimit`    | Application revision limit                                                                    | `10`                 |
| `definitionRevisionLimit`     | Definition revision limit                                                                     | `20`                 |
| `concurrentReconciles`        | concurrentReconciles is the concurrent reconcile number of the controller                     | `4`                  |
| `controllerArgs.reSyncPeriod` | The period for resync the applications                                                        | `5m`                 |
| `OAMSpecVer`                  | OAMSpecVer is the oam spec version controller want to setup                                   | `minimal`            |
| `disableCaps`                 | Disable capability                                                                            | `envbinding,rollout` |
| `dependCheckWait`             | dependCheckWait is the time to wait for ApplicationConfiguration's dependent-resource ready   | `30s`                |

### KubeVela workflow parameters

| Name                                   | Description                                            | Value   |
| -------------------------------------- | ------------------------------------------------------ | ------- |
| `workflow.enableSuspendOnFailure`      | Enable suspend on workflow failure                     | `false` |
| `workflow.backoff.maxTime.waitState`   | The max backoff time of workflow in a wait condition   | `60`    |
| `workflow.backoff.maxTime.failedState` | The max backoff time of workflow in a failed condition | `300`   |
| `workflow.step.errorRetryTimes`        | The max retry times of a failed workflow step          | `10`    |

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

### KubeVela controller optimization parameters

| Name                     | Description                                                                                                                           | Value   |
| ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------- | ------- |
| `featureGates.applyOnce` | if enabled, the apply-once feature will be applied to all applications, no state-keep and no resource data storage in ResourceTracker | `false` |

### MultiCluster parameters

| Name                                                    | Description                      | Value                            |
| ------------------------------------------------------- | -------------------------------- | -------------------------------- |
| `multicluster.enabled`                                  | Whether to enable multi-cluster  | `true`                           |
| `multicluster.clusterGateway.replicaCount`              | ClusterGateway replica count     | `1`                              |
| `multicluster.clusterGateway.port`                      | ClusterGateway port              | `9443`                           |
| `multicluster.clusterGateway.image.repository`          | ClusterGateway image repository  | `oamdev/cluster-gateway`         |
| `multicluster.clusterGateway.image.tag`                 | ClusterGateway image tag         | `v1.8.0-alpha.3`                 |
| `multicluster.clusterGateway.image.pullPolicy`          | ClusterGateway image pull policy | `IfNotPresent`                   |
| `multicluster.clusterGateway.resources.requests.cpu`    | ClusterGateway cpu request       | `50m`                            |
| `multicluster.clusterGateway.resources.requests.memory` | ClusterGateway memory request    | `20Mi`                           |
| `multicluster.clusterGateway.resources.limits.cpu`      | ClusterGateway cpu limit         | `500m`                           |
| `multicluster.clusterGateway.resources.limits.memory`   | ClusterGateway memory limit      | `200Mi`                          |
| `multicluster.clusterGateway.secureTLS.enabled`         | Whether to enable secure TLS     | `true`                           |
| `multicluster.clusterGateway.secureTLS.certPath`        | Path to the certificate file     | `/etc/k8s-cluster-gateway-certs` |

### Test parameters

| Name                  | Description         | Value                |
| --------------------- | ------------------- | -------------------- |
| `test.app.repository` | Test app repository | `oamdev/hello-world` |
| `test.app.tag`        | Test app tag        | `v1`                 |
| `test.k8s.repository` | Test k8s repository | `oamdev/alpine-k8s`  |
| `test.k8s.tag`        | Test k8s tag        | `1.18.2`             |

### Common parameters

| Name                          | Description                                                                                                                | Value                |
| ----------------------------- | -------------------------------------------------------------------------------------------------------------------------- | -------------------- |
| `imagePullSecrets`            | Image pull secrets                                                                                                         | `[]`                 |
| `nameOverride`                | Override name                                                                                                              | `""`                 |
| `fullnameOverride`            | Fullname override                                                                                                          | `""`                 |
| `serviceAccount.create`       | Specifies whether a service account should be created                                                                      | `true`               |
| `serviceAccount.annotations`  | Annotations to add to the service account                                                                                  | `{}`                 |
| `serviceAccount.name`         | The name of the service account to use. If not set and create is true, a name is generated using the fullname template     | `nil`                |
| `nodeSelector`                | Node selector                                                                                                              | `{}`                 |
| `tolerations`                 | Tolerations                                                                                                                | `[]`                 |
| `affinity`                    | Affinity                                                                                                                   | `{}`                 |
| `rbac.create`                 | Specifies whether a RBAC role should be created                                                                            | `true`               |
| `logDebug`                    | Enable debug logs for development purpose                                                                                  | `false`              |
| `logFilePath`                 | If non-empty, write log files in this path                                                                                 | `""`                 |
| `logFileMaxSize`              | Defines the maximum size a log file can grow to. Unit is megabytes. If the value is 0, the maximum file size is unlimited. | `1024`               |
| `kubeClient.qps`              | The qps for reconcile clients, default is 50                                                                               | `50`                 |
| `kubeClient.burst`            | The burst for reconcile clients, default is 100                                                                            | `100`                |
| `authentication.enabled`      | Enable authentication for application                                                                                      | `false`              |
| `authentication.withUser`     | Application authentication will impersonate as the request User                                                            | `false`              |
| `authentication.defaultUser`  | Application authentication will impersonate as the User if no user provided in Application                                 | `kubevela:vela-core` |
| `authentication.groupPattern` | Application authentication will impersonate as the request Group that matches the pattern                                  | `kubevela:*`         |


