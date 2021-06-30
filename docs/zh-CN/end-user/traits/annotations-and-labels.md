---
title: 使用 Labels and Annotations
---


## 列出 trait

`label` 和 `annotations` trait 允许您将标签和注释附加到组件。

```shell
# myapp.yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: myapp
spec:
  components:
    - name: express-server
      type: webservice
      properties:
        image: crccheck/hello-world
        port: 8000
      traits:
        - type: labels
          properties:
            "release": "stable"
        - type: annotations
          properties:
            "description": "web application"
```

部署这个 application.

```shell
kubectl apply -f myapp.yaml
```

在运行时集群上，检查工作负载是否已成功创建。

```bash
kubectl get deployments
```
```console
NAME             READY   UP-TO-DATE   AVAILABLE   AGE
express-server   1/1     1            1           15s
```

检查 `labels`.

```bash
kubectl get deployments express-server -o jsonpath='{.spec.template.metadata.labels}'
```
```console
{"app.oam.dev/component":"express-server","release": "stable"}
```

检查 `annotations`.

```bash
kubectl get deployments express-server -o jsonpath='{.spec.template.metadata.annotations}'
```
```console
{"description":"web application"}
```
