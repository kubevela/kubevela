# Web Service

## Description

`Web Service` is a workload type to describe long-running, scalable, containerized services that have a stable network endpoint to receive external network traffic from customers.

If workload type is skipped for any service defined in Appfile, it will be defaulted to `Web Service` type.

## Specification

List of all configuration options for a `Web Service` workload type.

```yaml
name: my-app-name

services:
  my-service-name:
    type: webservice # could be skipped
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

## Parameters

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Cmd** | **[]string** |  | [optional]
**Env** | [**[]WebserviceEnv**](#webserviceenv) |  | [optional]
**Image** | **string** | Which image would you like to use for your service |
**Port** | **int32** | Which port do you want customer traffic sent to | [default to 80]
**cpu** | **string** | Number of CPU units for the service, like `0.5` (0.5 CPU core), `1` (1 CPU core) | [optional]

### WebserviceEnv

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Name** | **string** |  |
**Value** | **string** |  | [optional]
**ValueFrom** | [**WebserviceValueFrom**](#webservicevaluefrom) |  | [optional]


### WebserviceValueFrom

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**SecretKeyRef** | [**WebserviceValueFromSecretKeyRef**](#webservicevaluefromsecretkeyref) |  |

### WebserviceValueFromSecretKeyRef

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Name** | **string** |  |
**Key** | **string** |  |