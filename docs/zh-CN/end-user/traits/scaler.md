---
title: 手动扩缩容
---

`scaler` trait 允许你手动扩缩容你的组件实例。

```shell
kubectl vela show scaler 
```
```console
# Properties
+----------+--------------------------------+------+----------+---------+
|   NAME   |          DESCRIPTION           | TYPE | REQUIRED | DEFAULT |
+----------+--------------------------------+------+----------+---------+
| replicas | Specify replicas of workload   | int  | true     |       1 |
+----------+--------------------------------+------+----------+---------+
```

声明一个具有缩放 trait 的 application。

```yaml
# sample-manual.yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: website
spec:
  components:
    - name: frontend
      type: webservice
      properties:
        image: nginx
      traits:
        - type: scaler
          properties:
            replicas: 2
        - type: sidecar
          properties:
            name: "sidecar-test"
            image: "fluentd"
    - name: backend
      type: worker
      properties:
        image: busybox
        cmd:
          - sleep
          - '1000'
```

应用示例 application：

```shell
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/docs/examples/enduser/sample-manual.yaml
```
```console
application.core.oam.dev/website configured
```

在运行时集群中，你可以看到 `frontend` 组件的底层部署现在有2个副本。

```shell
kubectl get deploy -l app.oam.dev/name=website
```
```console
NAME       READY   UP-TO-DATE   AVAILABLE   AGE
backend    1/1     1            1           19h
frontend   2/2     2            2           19h
```

要扩容或缩容，您只需要修改`scaler` trait 的`replicas` 字段并重新应用YAML。
