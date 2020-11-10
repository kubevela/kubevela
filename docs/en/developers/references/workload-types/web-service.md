# Webservice

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Cmd** | **[]string** |  | [optional]
**Env** | [**[]WebserviceEnv**](#webserviceenv) |  | [optional]
**Image** | **string** | specify app image |
**Port** | **int32** | specify port for container | [default to 6379]


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