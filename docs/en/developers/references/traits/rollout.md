---
title:  Rollout
---

## Description

Configures Canary deployment strategy for your application.

## Specification

List of all configuration options for a `Rollout` trait.

```yaml
...
    rollout:
      replicas: 2
      stepWeight: 50
      interval: "10s"
```

## Properties

Name | Description | Type | Required | Default 
------------ | ------------- | ------------- | ------------- | ------------- 
 interval | Schedule interval time | string | true | 30s 
 stepWeight | Weight percent of every step in rolling update | int | true | 50 
 replicas | Total replicas of the workload | int | true | 2 

## Conflicts With

### `Autoscale`

When `Rollout` and `Autoscle` traits are attached to the same service, they two will fight over the number of instances during rollout. Thus, it's by design that `Rollout` will take over replicas control (specified by `.replicas` field) during rollout.

> Note: in up coming releases, KubeVela will introduce a separate section in Appfile to define release phase configurations such as `Rollout`.
