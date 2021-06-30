---
title: Rollout Plan
---
在本文档中，我们将展示如何使用推出计划来滚动更新应用程序。

## 概述

默认情况下，当我们更新应用程序的属性时，KubeVela 会直接更新底层实例。 应用程序的可用性将通过 rollout trait（如果有）来保证。

虽然 KubeVela 也提供了滚动样式更新机制，但您可以在应用程序中指定`spec.rolloutPlan` 来实现。

## 例子

1. 将应用程序部署到集群
  ```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: test-rolling
spec:
  components:
    - name: metrics-provider
      type: worker
      properties:
        cmd:
          - ./podinfo
          - stress-cpu=1.0
        image: stefanprodan/podinfo:4.0.6
        port: 8080
  rolloutPlan:
    rolloutStrategy: "IncreaseFirst"
    rolloutBatches:
      - replicas: 50%
      - replicas: 50%
    targetSize: 6
  ```

2. 用户可以修改应用容器命令并申请
  ```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: test-rolling
spec:
  components:
    - name: metrics-provider
      type: worker
      properties:
        cmd:
          - ./podinfo
          - stress-cpu=2.0
        image: stefanprodan/podinfo:4.0.6
        port: 8080
  rolloutPlan:
    rolloutStrategy: "IncreaseFirst"
    rolloutBatches:
      - replicas: 50%
      - replicas: 50%
    targetSize: 6
  ```

用户可以检查应用程序的状态并看到部署完成，以及应用程序的 `status.rollout.rollingState` 变为 `rolloutSucceed`。





