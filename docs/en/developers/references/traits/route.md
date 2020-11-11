# Route

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Domain** | **string** | specify your host url for this app | [ default to (empty) ]
**Issuer** | **string** | specify your certificate issue  | [default to no tls]
**Rules** | [**[]RouteRules**](#routerules) |  | [optional] 


### RouteRules

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Path** | **string** |  | [ default to (empty) ]
**RewriteTarget** | **string** |  | [ default to (empty) ]
