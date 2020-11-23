# Route

## Description

`Route` is used to configure external access to your service.

## Specification

List of all available properties for a `Route` trait.

```yaml
name: my-app-name

services:
  my-service-name:
    ...
    route:
      domain: example.com
      issuer: tls
      rules:
        - path: /testapp
          rewriteTarget: /
```

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Domain** | **string** | specify your host url for this app | [ default to (empty) ]
**Issuer** | **string** | specify your certificate issue  | [default to no tls]
**Rules** | [**[]RouteRules**](#routerules) |  | [optional] 
**Provider** | **string** | ingress controller name ,we support **nginx**,**[contour](#https://github.com/projectcontour/contour)**  | [empty means using nginx-ingress]


### RouteRules

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Path** | **string** |  | [ default to (empty) ]
**RewriteTarget** | **string** |  | [ default to (empty) ]
