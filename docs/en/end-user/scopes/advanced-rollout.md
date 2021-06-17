---
title: Advanced Rollout Plan
---

The rollout plan feature in KubeVela is essentially provided by `AppRollout` API.

## AppRollout Specification

The following describes all the available fields of a AppRollout:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: AppRollout
metadata:
  name: rolling-example
spec:
  # SourceAppRevisionName contains the name of the appRevisionName that we need to upgrade from.
  # it can be empty only when you want to scale an  application. +optional
  sourceAppRevisionName: test-rolling-v1
  
  # TargetAppRevisionName contains the name of the appRevisionName that we need to upgrade to.
  targetAppRevisionName: test-rolling-v2
  
  # The list of component to upgrade in the application.
  # We only support single component application so far. +optional
  componentList:
    - metrics-provider
  # RolloutPlan is the details on how to rollout the resources
  rolloutPlan:
    
    # RolloutStrategy defines strategies for the rollout plan
    # the value can be IncreaseFirst or DecreaseFirst
    # Defaults to IncreaseFirst. +optional
    rolloutStrategy: "IncreaseFirst"
    
    # The exact distribution among batches.
    # its size has to be exactly the same as the NumBatches (if set)
    # The total number cannot exceed the targetSize or the size of the source resource
    # We will IGNORE the last batch's replica field if it's a percentage since round errors can lead to inaccurate sum
    # We highly recommend to leave the last batch's replica field empty
    rolloutBatches:
        
      # Replicas is the number of pods to upgrade in this batch
      # it can be an absolute number (ex: 5) or a percentage of total pods
      # we will ignore the percentage of the last batch to just fill the gap
      # Below is an example the first batch contains only 1 pod while the rest of the batches split the rest.
      - replicas: 1
      - replicas: 50%
      - replicas: 50%
        
    # All pods in the batches up to the batchPartition (included) will have
    # the target resource specification while the rest still have the source resource
    # This is designed for the operators to manually rollout
    # Default is the the number of batches which will rollout all the batches. +optional
    batchPartition: 1

    # Paused the rollout
    # defaults to false. +optional
    paused: false

    # The size of the target resource. In rollout operation it's the same as the size of the source resource.
    # when use rollout to scale an application targetSize is the target source you want scale to.  +optional
    targetSize: 4
```

## Basic Usage

1. Deploy application
    ```yaml
    apiVersion: core.oam.dev/v1beta1
    kind: Application
    metadata:
      name: test-rolling
      annotations:
        "app.oam.dev/rolling-components": "metrics-provider"
        "app.oam.dev/rollout-template": "true"
    spec:
      components:
        - name: metrics-provider
          type: worker
          properties:
            cmd:
              - ./podinfo
              - stress-cpu=1
            image: stefanprodan/podinfo:4.0.6
            port: 8080
            replicas: 5
    ```
    Verify AppRevision `test-rolling-v1` have generated
    ```shell
    $ kubectl get apprev test-rolling-v1
    NAME              AGE
    test-rolling-v1   9s
    ```

2. Attach the following rollout plan to upgrade the application to v1
    ```yaml
    apiVersion: core.oam.dev/v1beta1
    kind: AppRollout
    metadata:
      name: rolling-example
    spec:
      # application (revision) reference
      targetAppRevisionName: test-rolling-v1
      componentList:
        - metrics-provider
      rolloutPlan:
        rolloutStrategy: "IncreaseFirst"
        rolloutBatches:
          - replicas: 10%
          - replicas: 40%
          - replicas: 50%
        targetSize: 5
    ```
    User can check the status of the ApplicationRollout and wait for the rollout to complete.

3. User can continue to modify the application image tag and apply.This will generate new AppRevision `test-rolling-v2`
    ```yaml
    apiVersion: core.oam.dev/v1beta1
    kind: Application
    metadata:
      name: test-rolling
      annotations:
        "app.oam.dev/rolling-components": "metrics-provider"
        "app.oam.dev/rollout-template": "true"
    spec:
      components:
        - name: metrics-provider
          type: worker
          properties:
            cmd:
              - ./podinfo
              - stress-cpu=1
            image: stefanprodan/podinfo:5.0.2
            port: 8080
            replicas: 5
    ```

    Verify AppRevision `test-rolling-v2` have generated
    ```shell
    $ kubectl get apprev test-rolling-v2
    NAME              AGE
    test-rolling-v2   7s
    ```

4. Apply the application rollout that upgrade the application from v1 to v2
    ```yaml
    apiVersion: core.oam.dev/v1beta1
    kind: AppRollout
    metadata:
      name: rolling-example
    spec:
      # application (revision) reference
      sourceAppRevisionName: test-rolling-v1
      targetAppRevisionName: test-rolling-v2
      componentList:
        - metrics-provider
      rolloutPlan:
        rolloutStrategy: "IncreaseFirst"
        rolloutBatches:
          - replicas: 1
          - replicas: 2
          - replicas: 2
    ```
    User can check the status of the ApplicationRollout and see the rollout completes, and the
    ApplicationRollout's "Rolling State" becomes `rolloutSucceed`

## Advanced Usage

Using `AppRollout` separately can enable some advanced use case.

### Revert

5. Apply the application rollout that revert the application from v2 to v1

    ```yaml
    apiVersion: core.oam.dev/v1beta1
      kind: AppRollout
      metadata:
        name: rolling-example
      spec:
        # application (revision) reference
        sourceAppRevisionName: test-rolling-v2
        targetAppRevisionName: test-rolling-v1
        componentList:
          - metrics-provider
        rolloutPlan:
          rolloutStrategy: "IncreaseFirst"
          rolloutBatches:
            - replicas: 1
            - replicas: 2
            - replicas: 2
    ```

### Skip Revision Rollout

6. User can apply this yaml continue to modify the application image tag.This will generate new AppRevision `test-rolling-v3`
    ```yaml
      apiVersion: core.oam.dev/v1beta1
      kind: Application
      metadata:
        name: test-rolling
        annotations:
          "app.oam.dev/rolling-components": "metrics-provider"
          "app.oam.dev/rollout-template": "true"
      spec:
        components:
          - name: metrics-provider
            type: worker
            properties:
              cmd:
                - ./podinfo
                - stress-cpu=1
              image: stefanprodan/podinfo:5.2.0
              port: 8080
              replicas: 5
    ```

    Verify AppRevision `test-rolling-v3` have generated
    ```shell
    $ kubectl get apprev test-rolling-v3
    NAME              AGE
    test-rolling-v3   7s
    ```

7. Apply the application rollout that rollout the application from v1 to v3
    ```yaml
    apiVersion: core.oam.dev/v1beta1
      kind: AppRollout
      metadata:
        name: rolling-example
      spec:
        # application (revision) reference
        sourceAppRevisionName: test-rolling-v1
        targetAppRevisionName: test-rolling-v3
        componentList:
          - metrics-provider
        rolloutPlan:
          rolloutStrategy: "IncreaseFirst"
          rolloutBatches:
            - replicas: 1
            - replicas: 2
            - replicas: 2
    ```

### Scale the application

Before using AppRollout to scale an application, we must be aware of the real status of workload now. Check the workload status. 

```shell
$ kubectl get deploy metrics-provider-v3
 NAME                  READY   UP-TO-DATE   AVAILABLE   AGE
 metrics-provider-v3   5/5     5            5           10m
```

Last target appRevision is `test-rolling-v3` and the workload have 5 replicas currently.

8. Apply the appRollout increase the replicas nums of workload to 7.
    ```yaml
    apiVersion: core.oam.dev/v1beta1
    kind: AppRollout
    metadata:
      name: rolling-example
    spec:
      # sourceAppRevisionName is empty means this is a scale operation
      targetAppRevisionName: test-rolling-v3
      componentList:
      - metrics-provider
      rolloutPlan:
         rolloutStrategy: "IncreaseFirst"
         rolloutBatches:
         # split two batches to scale. First batch increase 1 pod and second increase 1.
           - replicas: 1
           - replicas: 1
         # targetSize means that final total size of workload is 7
         targetSize: 7
    ```

## More Details About `AppRollout`

### Design Principles and Goals

There are several attempts at solving rollout problem in the cloud native community. However, none
of them provide a true rolling style upgrade. For example, flagger supports Blue/Green, Canary
and A/B testing. Therefore, we decide to add support for batch based rolling upgrade as
our first style to support in KubeVela.

We design KubeVela rollout solutions with the following principles in mind
- First, we want all flavors of rollout controllers share the same core rollout
  related logic. The trait and application related logic can be easily encapsulated into its own
  package.
- Second, the core rollout related logic is easily extensible to support different type of
  workloads, i.e. Deployment, CloneSet, Statefulset, DaemonSet or even customized workloads.
- Thirdly, the core rollout related logic has a well documented state machine that
  does state transition explicitly.
- Finally, the controllers can support all the rollout/upgrade needs of an application running
  in a production environment including Blue/Green, Canary and A/B testing.

### State Transition
Here is the high level state transition graph

![](../../resources/approllout-status-transition.jpg)

### Roadmap

Our recent roadmap for rollout plan is [here](./roadmap).
