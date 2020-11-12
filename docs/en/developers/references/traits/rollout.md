# Rollout

## Description

`Rollout` is used to configure Canary rollout strategy to your application.

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
**replica**** | **string** | total replica for your app | [ default to 5 ]
**stepWeight** | **string** | weight percent of every step for this update | [default to 20 ]
**interval** | **string** | wait interval for every rolling update step | [default to '30s'] 
