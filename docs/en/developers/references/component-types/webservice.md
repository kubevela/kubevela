---
title:  Webservice
---

## Description

Describes long-running, scalable, containerized services that have a stable network endpoint to receive external network traffic from customers. If workload type is skipped for any service defined in Appfile, it will be defaulted to `webservice` type.

## Specification

List of all configuration options for a `Webservice` workload type.

```yaml
...
    image: oamdev/testapp:v1
    cmd: ["node", "server.js"]
    port: 8080
    cpu: "0.1"
    env:
      - name: FOO
        value: bar
      - name: FOO
        valueFrom:
          secretKeyRef:
            name: bar
            key: bar
```

## Properties

Name | Description | Type | Required | Default 
------------ | ------------- | ------------- | ------------- | ------------- 
 cmd | Commands to run in the container | []string | false |  
 env | Define arguments by using environment variables | [[]env](#env) | false |  
 image | Which image would you like to use for your service | string | true |  
 port | Which port do you want customer traffic sent to | int | true | 80 
 cpu | Number of CPU units for the service, like `0.5` (0.5 CPU core), `1` (1 CPU core) | string | false |  


### env

Name | Description | Type | Required | Default 
------------ | ------------- | ------------- | ------------- | ------------- 
 name | Environment variable name | string | true |  
 value | The value of the environment variable | string | false |  
 valueFrom | Specifies a source the value of this var should come from | [valueFrom](#valueFrom) | false |  


#### valueFrom

Name | Description | Type | Required | Default 
------------ | ------------- | ------------- | ------------- | ------------- 
 secretKeyRef | Selects a key of a secret in the pod's namespace | [secretKeyRef](#secretKeyRef) | true |  


##### secretKeyRef

Name | Description | Type | Required | Default 
------------ | ------------- | ------------- | ------------- | ------------- 
 name | The name of the secret in the pod's namespace to select from | string | true |  
 key | The key of the secret to select from. Must be a valid secret key | string | true |  
