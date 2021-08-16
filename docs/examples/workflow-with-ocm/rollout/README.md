# Rollout with OCM

In this tutorial, you will use rollout in runtime cluster with OCM.

## Prerequisites

- Have a multi-cluster environment witch have installed OCM by following this [guid](../README.md). The name of  managed cluster generally is `poc-01`.

- install vela-rollout chart in managed-cluster with Helm.
```shell
$ helm repo add kubevela https://charts.kubevela.net/core
```
```shell
$ helm isntall vela-rollout  --create-namespace -n vela-system kubevela/vela-rollout
```

## Install workflowStepDefinition

Apply workflowStepDefinitions in control-plane cluster.

```shell
$ kubectl apply -f dispatchRevDef.yaml
```

```shell
$ kubectl apply -f dispatchTraits.yaml
```

## Deploy and rollout

1. Apply application in control-plane cluster.

```shell
$ kubectl apply -f app-first-scale.yaml
```

Wait a few minute, check rollout and workload status in managed cluster.
```shell
$ kubectl get rollout
NAME           TARGET   UPGRADED   READY   BATCH-STATE   ROLLING-STATE    AGE
nginx-server   2        2          2       batchReady    rolloutSucceed   25h
```
```shell
$ kubectl get deploy
NAME              READY   UP-TO-DATE   AVAILABLE   AGE
nginx-server-v1   2/2     2            2           25h
```

2. Update application to v2.

```shell
$ kubectl apply -f app-update-v2.yaml
```

check rollout and workload status.
```shell
$ kubectl get rollout
NAME           TARGET   UPGRADED   READY   BATCH-STATE   ROLLING-STATE    AGE
nginx-server   2        2          2       batchReady    rolloutSucceed   25h
```
```shell
$ kubectl get deploy
NAME              READY   UP-TO-DATE   AVAILABLE   AGE
nginx-server-v2   2/2     2            2           25h
```

3. Scale up.

```shell
$ kubectl apply -f app-v2-scale.yaml
```

check the status of rollout and workload.
```shell
$ kubectl get rollout
NAME           TARGET   UPGRADED   READY   BATCH-STATE   ROLLING-STATE    AGE
nginx-server   4        4          4       batchReady    rolloutSucceed   25h
```
```shell
$ kubectl get deploy
NAME              READY   UP-TO-DATE   AVAILABLE   AGE
nginx-server-v2   4/4     4            4           25h
```

4. Roll back to v1.
```shell
$ kubectl apply -f app-revert.yaml
```

check rollout and workload status.
```shell
$ kubectl get rollout
NAME           TARGET   UPGRADED   READY   BATCH-STATE   ROLLING-STATE    AGE
nginx-server   4        4          4       batchReady    rolloutSucceed   25h
```

```shell
$ kubectl get deploy
NAME              READY   UP-TO-DATE   AVAILABLE   AGE
nginx-server-v1   4/4     4            4           25h
```

5. Scale down.
```shell
$ kubectl apply -f app-scale-down-v1.yaml
```

check rollout and workload status.
```shell
$ kubectl get rollout
NAME           TARGET   UPGRADED   READY   BATCH-STATE   ROLLING-STATE    AGE
nginx-server   2        2          2       batchReady    rolloutSucceed   25h
```

```shell
$ kubectl get deploy
NAME              READY   UP-TO-DATE   AVAILABLE   AGE
nginx-server-v1   2/2     2            2           25h
```