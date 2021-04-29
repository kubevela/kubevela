---
title: Update workload by rollout
---
In this documentation, we will show how to use Progressive Rollout to update application

## Overview

By default, when update components of application kubevela will update workload directly.This will cause kill all pods at once.You can progressively update you workload pods by setting `spec.rolloutPlan` in application

## User Workflow
Here is the end to end user experience based on [CloneSet](https://openkruise.io/en-us/docs/cloneset.html)

1. Install CloneSet and its `ComponentDefinition`.

Since CloneSet is an customized workload for Kubernetes, we need to install its controller and component definition manually to KubeVela platform.

  ```shell
  helm install kruise https://github.com/openkruise/kruise/releases/download/v0.7.0/kruise-chart.tgz
  ```

  ```shell
  kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/docs/examples/cloneset-rollout/clonesetDefinition.yaml
  ```

2. Deploy application to the cluster
  ```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: test-rolling
spec:
  components:
    - name: metrics-provider
      type: clonesetservice
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

3. User can  modify the application container command and apply
  ```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: test-rolling
spec:
  components:
    - name: metrics-provider
      type: clonesetservice
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

User can check the status of the Application and see the rollout completes, and the
Application's `status.rollout.rollingState` becomes `rolloutSucceed`




