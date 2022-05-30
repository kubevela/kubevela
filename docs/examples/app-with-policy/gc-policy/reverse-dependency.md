# How to garbage collect resources in the order of dependency

If you want to garbage collect resources in the order of reverse dependency, you can add `order: dependency` in the `garbage-collect` policy.

> Notice that this order policy is only valid for the resources that are created in the components.

In the following example, component `test1` depends on `test2`, and `test2` need the output from `test3`.

So the order of deployment is: `test3 -> test2 -> test1`.

When we add `order: dependency` in `garbage-collect` policy and delete the application, the order of garbage collect is: `test1 -> test2 -> test3`.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: gc-dependency
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
    - name: gc-dependency
      type: garbage-collect
      properties:
        order: dependency
```
