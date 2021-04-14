---
title:  Generating UI Forms
---

For any capabilities installed via [Definition Objects](./definition-and-templates),
KubeVela will automatically generate OpenAPI v3 JSON schema based on its parameter list, and store it in a `ConfigMap` in the same `namespace` with the definition object. 

> The default KubeVela system `namespace` is `vela-system`, the built-in capabilities and schemas are laid there.


## List Schema

This `ConfigMap` will have a common label `definition.oam.dev=schema`, so you can find easily by:

```shell
$ kubectl get configmap -n vela-system -l definition.oam.dev=schema
NAME                DATA   AGE
schema-ingress      1      19s
schema-scaler       1      19s
schema-task         1      19s
schema-webservice   1      19s
schema-worker       1      20s
```

The `ConfigMap` name is in the format of `schema-<your-definition-name>`,
and the data key is `openapi-v3-json-schema`.

For example, we can use the following command to get the JSON schema of `webservice`.

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

Specifically, this schema is generated based on `parameter` section in capability definition:

* For CUE based definition: the [`parameter`](../cue/component#Write-ComponentDefinition) is a keyword in CUE template.
* For Helm based definition: the [`parameter`](../helm/component#Write-ComponentDefinition) is generated from `values.yaml` in Helm chart.

## Render Form

You can render above schema into a form by [form-render](https://github.com/alibaba/form-render) or [React JSON Schema form](https://github.com/rjsf-team/react-jsonschema-form) and integrate with your dashboard easily.

Below is a form rendered with `form-render`:

![](../resources/json-schema-render-example.jpg)

# What's Next

It's by design that KubeVela supports multiple ways to define the schematic. Hence, we will explain `.schematic` field in detail with following guides.
