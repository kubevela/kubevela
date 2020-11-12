# Worker

## Description

`Worker` is a workload type to describe long-running, scalable, containerized services that running at backend. They do NOT have network endpoint to receive external network traffic.

## Specification

List of all configuration options for a `Worker` workload type.

```yaml
name: my-app-name

services:
  my-service-name:
    type: worker
    image: oamdev/testapp:v1
    cmd: ["node", "server.js"]
```

## Parameters

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Cmd** | **[]string** |  | [optional] 
**Image** | **string** | pecify app image | 
