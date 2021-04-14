---
title:  Ingress
---

## Description

Configures K8s ingress and service to enable web traffic for your service. Please use route trait in cap center for advanced usage.

## Specification

List of all configuration options for a `Ingress` trait.

```yaml
...
    domain: testsvc.example.com
    http:
      /: 8000
```

## Properties

Name | Description | Type | Required | Default 
------------ | ------------- | ------------- | ------------- | ------------- 
 domain |  | string | true |  
 http |  | map[string]int | true |  
