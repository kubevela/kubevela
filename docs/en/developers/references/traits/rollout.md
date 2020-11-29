# Rollout

## Description

`Rollout` is used to configure Canary deployment strategy to your application.

## Specification

List of all configuration options for a `Rollout` trait.

```yaml
servcies:
  express-server:
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
