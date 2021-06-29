---
title: Rollout Plan
---
In this documentation, we will show how to use the rollout plan to rolling update an application.

## Overview

By default, when we update the properties of application, KubeVela will update the underlying instances directly. The availability of the application will be guaranteed by rollout traits (if any).

Though KubeVela also provides a rolling style update mechanism, you can specify the `spec.rolloutPlan` in application to do so.

## Example

1. Deploy application to the cluster
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

2. User can  modify the application container command and apply
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

User can check the status of the application and see the rollout completes, and the
application's `status.rollout.rollingState` becomes `rolloutSucceed`.




