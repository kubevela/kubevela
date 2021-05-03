---
title:  Metrics
---

## Description

Configures monitoring metrics for your service.

## Specification

List of all configuration options for a `Metrics` trait.

```yaml
...
    format: "prometheus"
    port: 8080
    path: "/metrics"
    scheme:  "http"
    enabled: true
```

## Properties

Name | Description | Type | Required | Default 
------------ | ------------- | ------------- | ------------- | ------------- 
 path | The metrics path of the service | string | true | /metrics 
 format | Format of the metrics, default as prometheus | string | true | prometheus 
 scheme | The way to retrieve data which can take the values `http` or `https` | string | true | http 
 enabled |  | bool | true | true 
 port | The port for metrics, will discovery automatically by default | int | true | 0 
 selector | The label selector for the pods, will discovery automatically by default | map[string]string | false |  
