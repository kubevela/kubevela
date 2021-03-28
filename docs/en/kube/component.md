# Use Raw Kubernetes Resource To Extend a Component type

This documentation explains how to use raw K8s resource to define an application component.

Before reading this part, please make sure you've learned [the definition and template concepts](../platform-engineers/definition-and-templates.md).

## Write ComponentDefinition

Here is an example `ComponentDefinition` about how to use raw k8s resource as schematic module.

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

Just like using CUE as schematic module, we also have some rules and contracts
to use raw k8s resource as schematic module.
`.spec.schematic.kube` contains template of the raw k8s resource and
configurable parameters.

- `.spec.schematic.kube.template` is the raw k8s resource in YAML format just like
we usually defined in a YAML file.

- `.spec.schematic.kube.parameters` contains a set of configurable parameters.
`name`, `type`, and `fieldPaths` are required fields.
`description` and `required` are optional fields.
        
  - The parameter `name` must be unique in a `ComponentDefinition`.

  - `type` indicates the data type of value set to the field in a workload.
This is a required field which will help Vela to generate a OpenAPI JSON schema
for the parameters automatically. 
Currently, only basic data types are allowed, including `string`, `number`, and
`boolean`, while `array` and `object` are not.

  - `fieldPaths` in the parameter specifies an array of fields within this workload
that will be overwritten by the value of this parameter. 	
All fields must be of the same type. 
Fields are specified as JSON field paths without a leading dot, for example
`spec.replicas`, `spec.containers[0].image`.

## Create an Application using Kube schematic ComponentDefinition

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

Kube schematic workload will use data in `properties` as the values of
parameters.
Since parameters only support basic data type, values in `properties` should be
formatted as simple key-value, `<parameterName>: <parameterValue>`.
And don't forget to set value to required parameter.

Deploy the `Application` and verify the resulting workload.

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

