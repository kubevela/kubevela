# How to garbage collect resources in the order of reverse dependency

If you want to garbage collect resources in the order of reverse dependency, you can add `order: reverseDependency` in the `garbage-collect` policy.

> Notice that this order policy is only valid for the resources that are created in the components.

Example:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: gc-reverse-depends-on
  namespace: default
spec:
  components:
  - name: test1
    type: webservice
    properties:
      image: crccheck/hello-world
      port: 8000
    dependsOn:
      - "test2"
  - name: test2
    type: webservice
    properties:
      image: crccheck/hello-world
      port: 8000
    inputs:
      - from: test3-output
        parameterKey: test
  - name: test3
    type: webservice
    properties:
      image: crccheck/hello-world
      port: 8000
    outputs:
      - name: test3-output
        valueFrom: output.metadata.name
  
  policies:
    - name: reverse-dependency
      type: garbage-collect
      properties:
        order: reverseDependency
```
