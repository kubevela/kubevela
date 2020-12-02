# Scaler

## Description

`Scaler` is used to configure replicas for your service.

## Specification

List of all configuration options for a `Scaler` trait.

```yaml
name: my-app-name

services:
  my-service-name:
    ...
    scaler:
      replicas: 100
```

## Properties

Name | Description | Type | Required | Default 
------------ | ------------- | ------------- | ------------- | ------------- 
 replicas | Replicas of the workload | int | true | 1 
