---
title:  从定义中生成表单
---

对于任何通过[定义对象](./definition-and-templates) 安装的 capability, KubeVela 会自动根据 OpenAPI v3 JSON schema 的参数列表来生成 OpenAPI v3 JSON schema，并把它储存到一个和定义对象处于同一个 `namespace` 的 `ConfigMap` 中。

> 默认的 KubeVela 系统 `namespace` 是 `vela-system`，内置的 capability 和 schema 位于此处。

## 列出 Schema

这个 `ConfigMap` 会有一个通用的标签 `definition.oam.dev=schema`，所以你可以轻松地通过下述方法找到他们:

```shell
$ kubectl get configmap -n vela-system -l definition.oam.dev=schema
NAME                DATA   AGE
schema-ingress      1      19s
schema-scaler       1      19s
schema-task         1      19s
schema-webservice   1      19s
schema-worker       1      20s
```

`ConfigMap` 命名的格式为 `schema-<your-definition-name>`，数据键为 `openapi-v3-json-schema`.

举个例子，我们可以使用以下命令来获取 `webservice` 的JSON schema。

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

具体来说，该 schema 是根据capability 定义中的 `parameter` 部分生成的：

* 对于基于 CUE 的定义：`parameter` CUE 模板中的关键词。
* 对于基于 Helm 的定义：`parameter` 是从在 Helm Chart 中的 `values.yaml` 生成的。

## 渲染表单

你可以通过 [form-render](https://github.com/alibaba/form-render) 或者 [React JSON Schema form](https://github.com/rjsf-team/react-jsonschema-form) 渲染上述 schema 到表单中并轻松地集成到你的仪表盘中。

以下是使用 `form-render` 渲染的表单：

![](../resources/json-schema-render-example.jpg)

# 下一步

根据设计，KubeVela 支持多种方法来定义 schematic。因此，我们将在接下来的文档来详细解释 `.schematic` 字段。
