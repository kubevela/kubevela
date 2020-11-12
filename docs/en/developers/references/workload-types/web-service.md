# Webservice

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Cmd** | **[]string** |  | [optional]
**Env** | [**[]WebserviceEnv**](#webserviceenv) |  | [optional]
**Image** | **string** | Which image would you like to use for your service |
**Port** | **int32** | Which port do you want customer traffic sent to | [default to 80]
**CpuRequests** | **string** | CPU core requests for the workload, specify like &#39;0.5&#39;, &#39;1&#39 | [optional]

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