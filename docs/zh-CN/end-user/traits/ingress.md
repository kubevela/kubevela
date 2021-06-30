---
title: 使用 Ingress
---

> ⚠️ 本节要求您的运行时集群有一个有效的 ingress 控制器。

`ingress` trait 通过有效域将组件暴露给公共 Internet。

```shell
kubectl vela show ingress
```
```console
# Properties
+--------+------------------------------------------------------------------------------+----------------+----------+---------+
|  NAME  |                                 DESCRIPTION                                  |      TYPE      | REQUIRED | DEFAULT |
+--------+------------------------------------------------------------------------------+----------------+----------+---------+
| http   | Specify the mapping relationship between the http path and the workload port | map[string]int | true     |         |
| domain | Specify the domain you want to expose                                        | string         | true     |         |
+--------+------------------------------------------------------------------------------+----------------+----------+---------+
```

将 `ingress` trait 附加到要公开和部署的组件。

```yaml
# vela-app.yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: first-vela-app
spec:
  components:
    - name: express-server
      type: webservice
      properties:
        image: crccheck/hello-world
        port: 8000
      traits:
        - type: ingress
          properties:
            domain: testsvc.example.com
            http:
              "/": 8000
```

```bash
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/docs/examples/vela-app.yaml
```
```console
application.core.oam.dev/first-vela-app created
```

检查状态，直到我们看到 `status` 为 `running` 并且服务为 `healthy`：

```bash
kubectl get application first-vela-app -w
```
```console
NAME             COMPONENT        TYPE         PHASE            HEALTHY   STATUS   AGE
first-vela-app   express-server   webservice   healthChecking                      14s
first-vela-app   express-server   webservice   running          true               42s
```

检查其访问网址的 trait 详细信息：

```shell
kubectl get application first-vela-app -o yaml
```
```console
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: first-vela-app
  namespace: default
spec:
...
  services:
  - healthy: true
    name: express-server
    traits:
    - healthy: true
      message: 'Visiting URL: testsvc.example.com, IP: 47.111.233.220'
      type: ingress
  status: running
...
```

然后您将能够通过其域访问该应用程序。

```
curl -H "Host:testsvc.example.com" http://<your ip address>/
```
```console
<xmp>
Hello World


                                       ##         .
                                 ## ## ##        ==
                              ## ## ## ## ##    ===
                           /""""""""""""""""\___/ ===
                      ~~~ {~~ ~~~~ ~~~ ~~~~ ~~ ~ /  ===- ~~~
                           \______ o          _,/
                            \      \       _,'
                             `'--.._\..--''
</xmp>
```
