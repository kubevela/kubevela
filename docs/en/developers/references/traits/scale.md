# Scaler

## Description

`Scaler` is used to configure replicas to your service.

## Specification

List of all available properties for a `Scaler` trait.

```yaml
name: my-app-name

services:
  my-service-name:
    ...
    scaler:
      replicas: 100
```

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Replica** | **int32** | | [default to 1]
