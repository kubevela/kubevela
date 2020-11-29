# Route

## Description

`Route` is used to configure external access to your service.

## Specification

List of all configuration options for a `Route` trait.

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

Name | Description | Type | Required | Default 
------------ | ------------- | ------------- | ------------- | ------------- 
 domain |  Domain name | string | true | empty 
 issuer |  | string | true | empty 
 rules |  | [[]rules](#rules) | false |  


### rules

Name | Description | Type | Required | Default 
------------ | ------------- | ------------- | ------------- | ------------- 
 path |  | string | true |  
 rewriteTarget |  | string | true | empty 
 
