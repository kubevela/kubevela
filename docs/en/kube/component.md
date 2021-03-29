---
title:  How-to
---

In this section, it will introduce how to use raw K8s Object to declare app components via `ComponentDefinition`.

> Before reading this part, please make sure you've learned [the definition and template concepts](../platform-engineers/definition-and-templates).

## Declare `ComponentDefinition`

Here is a raw template based `ComponentDefinition` example which provides a abstraction for worker workload type:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: kube-worker
  namespace: default
spec:
  workload: 
    definition: 
      apiVersion: apps/v1
      kind: Deployment
  schematic:
    kube: 
      template:
        apiVersion: apps/v1
        kind: Deployment
        spec:
          selector:
            matchLabels:
              app: nginx
          template:
            metadata:
              labels:
                app: nginx
            spec:
              containers:
              - name: nginx
                ports:
                - containerPort: 80 
      parameters: 
      - name: image
        required: true
        type: string
        fieldPaths: 
        - "spec.template.spec.containers[0].image"
```

In detail, the `.spec.schematic.kube` contains template of a workload resource and
configurable parameters.
- `.spec.schematic.kube.template` is the raw template in YAML format.
- `.spec.schematic.kube.parameters` contains a set of configurable parameters. The `name`, `type`, and `fieldPaths` are required fields, `description` and `required` are optional fields.
  - The parameter `name` must be unique in a `ComponentDefinition`.
  - `type` indicates the data type of value set to the field. This is a required field which will help KubeVela to generate a OpenAPI JSON schema for the parameters automatically. In raw template, only basic data types are allowed, including `string`, `number`, and `boolean`, while `array` and `object` are not.
  - `fieldPaths` in the parameter specifies an array of fields within the template that will be overwritten by the value of this parameter. Fields are specified as JSON field paths without a leading dot, for example
`spec.replicas`, `spec.containers[0].image`.

## Declare an `Application`

Here is an example `Application`.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: myapp
  namespace: default
spec:
  components:
    - name: mycomp
      type: kube-worker
      properties: 
        image: nginx:1.14.0
```

Since parameters only support basic data type, values in `properties` should be simple key-value, `<parameterName>: <parameterValue>`.

Deploy the `Application` and verify the running workload instance.

```shell
$ kubectl get deploy
NAME                     READY   UP-TO-DATE   AVAILABLE   AGE
mycomp                   1/1     1            1           66m
```
And check the parameter works.
```shell
$ kubectl get deployment mycomp -o json | jq '.spec.template.spec.containers[0].image'
"nginx:1.14.0"
```

