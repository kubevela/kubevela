# Auto-generated Schema for Capability Parameters

For any installed capabilities from [definition files](./definition-and-templates.md),
KubeVela will automatically generate OpenAPI v3 JSON Schema for the parameters defined.
So end users can learn how to write the Application Object from it.

Platform builders can integrate the schema API to build a new UI for their end users.

## An integration workflow

In definition objects, `parameter` is always required as the entrance for encapsulation of the capabilities.

* CUE: the [`parameter`](../cue/component.md#Write-ComponentDefinition) is a `keyword` in CUE template.
* HELM: the [`parameter``](../helm/component.md#Write-ComponentDefinition) is generated from `values.yaml` in HELM chart.

When a new ComponentDefinition or TraitDefinition applied in K8s, KubeVela will watch the resources and 
generate a `ConfigMap` in the same namespace with the definition object.

The default KubeVela system namespace is `vela-system`, the built-in capabilities are laid there.

The ConfigMap will have a common label `definition.oam.dev=schema`, so you can find easily by:

```shell
$ kubectl get configmap -n vela-system -l definition.oam.dev=schema
NAME                DATA   AGE
schema-ingress      1      19s
schema-scaler       1      19s
schema-task         1      19s
schema-webservice   1      19s
schema-worker       1      20s
```

The ConfigMap name is in the format of `schema-<your-definition-name>`,
and the `key` of ConfigMap is `openapi-v3-json-schema`.

For example, we can use the following command to get the JSON Schema of `webservice`.

```shell
$ kubectl get configmap schema-webservice -n vela-system -o yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: schema-webservice
  namespace: vela-system
data:
  openapi-v3-json-schema: '{"properties":{"cmd":{"description":"Commands to run in
    the container","items":{"type":"string"},"title":"cmd","type":"array"},"cpu":{"description":"Number
    of CPU units for the service, like `0.5` (0.5 CPU core), `1` (1 CPU core)","title":"cpu","type":"string"},"env":{"description":"Define
    arguments by using environment variables","items":{"properties":{"name":{"description":"Environment
    variable name","title":"name","type":"string"},"value":{"description":"The value
    of the environment variable","title":"value","type":"string"},"valueFrom":{"description":"Specifies
    a source the value of this var should come from","properties":{"secretKeyRef":{"description":"Selects
    a key of a secret in the pod''s namespace","properties":{"key":{"description":"The
    key of the secret to select from. Must be a valid secret key","title":"key","type":"string"},"name":{"description":"The
    name of the secret in the pod''s namespace to select from","title":"name","type":"string"}},"required":["name","key"],"title":"secretKeyRef","type":"object"}},"required":["secretKeyRef"],"title":"valueFrom","type":"object"}},"required":["name"],"type":"object"},"title":"env","type":"array"},"image":{"description":"Which
    image would you like to use for your service","title":"image","type":"string"},"port":{"default":80,"description":"Which
    port do you want customer traffic sent to","title":"port","type":"integer"}},"required":["image","port"],"type":"object"}'
```

Then the platform builder can follow the [OpenAPI v3 Specification](https://github.com/OAI/OpenAPI-Specification/blob/master/versions/3.0.2.md#format)
to build their own GUI for end users. 

For example, you can render the schema by [form-render](https://github.com/alibaba/form-render) or [React JSON Schema form](https://github.com/rjsf-team/react-jsonschema-form).

A web form rendered from the `schema-webservice` can be as below.

![](../../resources/json-schema-render-example.jpg)
