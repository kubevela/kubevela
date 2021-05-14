---
title: Ingress
---

> ⚠️ This section requires your runtime cluster has a working ingress controller.

The `ingress` trait exposes a component to public Internet via a valid domain.

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

Attach a `ingress` trait to the component you want to expose and deploy.

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

Check the status until we see `status` is `running` and services are `healthy`:

```bash
kubectl get application first-vela-app -w
```
```console
NAME             COMPONENT        TYPE         PHASE            HEALTHY   STATUS   AGE
first-vela-app   express-server   webservice   healthChecking                      14s
first-vela-app   express-server   webservice   running          true               42s
```

Check the trait detail for the its visiting url:

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

Then you will be able to visit this application via its domain.

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
