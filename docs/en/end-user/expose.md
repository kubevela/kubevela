---
title: Expose Application
---

> ⚠️ This section requires your cluster has a working ingress.

To expose your application publicly, you just need to add an `ingress` trait.

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

Apply this application:

```bash
$ kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/docs/examples/vela-app.yaml
application.core.oam.dev/first-vela-app created
```

Check the status until we see `status` is `running` and services are `healthy`:

```bash
$ kubectl get application first-vela-app -w
NAME             COMPONENT        TYPE         PHASE            HEALTHY   STATUS   AGE
first-vela-app   express-server   webservice   healthChecking                      14s
first-vela-app   express-server   webservice   running          true               42s
```

You can also see the trait detail for the visiting url:

```shell
$ kubectl get application first-vela-app -o yaml
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

Then you will be able to visit this service.

```
$ curl -H "Host:testsvc.example.com" http://<your ip address>/
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
