# Metrics

## Description

`Metrics` is used to configure monitoring metrics to your service.

## Specification

List of all available properties for a `Route` trait.

```yaml
name: my-app-name

services:
  my-service-name:
    ...
    metrics:
      format: "prometheus"
      port: 8080
      path: "/metrics"
      scheme:  "http"
      enabled: true
```

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Path** | **string** | the metric path of the service | [default to /metrics]
**Format** | **string** | +format of the metrics, default as prometheus | [default to prometheus]
**Scheme** | **string** |  | [default to http]
**Enabled** | **bool** |  | [default to true]
**Port** | **int32** | the port for metrics, will discovery automatically by default | [default to 0], >=1024 & <=65535
**Selector** | **map[string]string** | the label selector for the pods, will discovery automatically by default | [optional] 
