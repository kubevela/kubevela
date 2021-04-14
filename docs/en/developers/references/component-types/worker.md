---
title:  Worker
---

## Description

Describes long-running, scalable, containerized services that running at backend. They do NOT have network endpoint to receive external network traffic.

## Specification

List of all configuration options for a `Worker` workload type.

```yaml
...
    image: oamdev/testapp:v1
    cmd: ["node", "server.js"]
```

## Properties

Name | Description | Type | Required | Default 
------------ | ------------- | ------------- | ------------- | ------------- 
 cmd | Commands to run in the container | []string | false |  
 image | Which image would you like to use for your service | string | true |  
