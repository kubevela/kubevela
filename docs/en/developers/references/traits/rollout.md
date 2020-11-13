# Rollout

## Description

`Rollout` is used to configure Canary deployment strategy to your application.

## Conflicts With

### `Autoscale`

When `Rollout` and `Autoscle` traits are attached to the same service, they two will fight over the number of instances during rollout. Thus, it's by design that `Rollout` will take over replicas control (specified by `.replica` field) during rollout.

> Note: in up coming releases, KubeVela will introduce a separate section in Appfile to define release phase configurations such as `Rollout`.

## Specification

List of all available properties for a `Rollout` trait.

```yaml
servcies:
  express-server:
    ...

    rollout:
      replica: 5
      stepWeight: 20
      interval: "30s"
```

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**replica** | **string** | replica number of the service instance per revision | [ default to 5 ]
**stepWeight** | **string** | canary increment step percentage (0-100)| [default to 20 ]
**interval** | **string** | wait interval for every rolling update step | [default to '30s'] 
