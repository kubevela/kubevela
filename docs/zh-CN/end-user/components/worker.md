---
title:  Worker
---

## 定义

描述在后端运行的长期运行、可扩展、容器化的服务。 他们没有网络端点来接收外部网络流量。

## 示例

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: app-worker
spec:
  components:
    - name: myworker
      type: worker
      properties:
        image: "busybox"
        cmd:
          - sleep
          - "1000"
```

## 属性说明

```console
# Properties
+-------+----------------------------------------------------+----------+----------+---------+
| NAME  |                    DESCRIPTION                     |   TYPE   | REQUIRED | DEFAULT |
+-------+----------------------------------------------------+----------+----------+---------+
| cmd   | Commands to run in the container                   | []string | false    |         |
| image | Which image would you like to use for your service | string   | true     |         |
+-------+----------------------------------------------------+----------+----------+---------+
``` 
