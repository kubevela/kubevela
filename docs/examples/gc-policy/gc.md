## How to use garbage-collect policy

Suppose you want to keep the resources created by the old version of the app. You only need to specify garbage-collect in the policy field of the app and enable the option `keepLegacyResource`.

```yaml
#app.yaml
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
        - type: ingress-1-20
          properties:
            domain: testsvc.example.com
            http:
              "/": 8000
  policies:
    - name: keep-legacy-resource
      type: garbage-collect
      properties:
        keepLegacyResource: true
```

1. create app

``` shell
kubectl apply -f app.yaml
```

```shell
$ kubectl get app
NAME             COMPONENT        TYPE         PHASE     HEALTHY   STATUS   AGE
first-vela-app   express-server   webservice   running   true               29s
```

2. update the app

```yaml
#app1.yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: first-vela-app
spec:
  components:
    - name: express-server-1
      type: webservice
      properties:
        image: crccheck/hello-world
        port: 8000
      traits:
        - type: ingress-1-20
          properties:
            domain: testsvc.example.com
            http:
              "/": 8000
  policies:
    - name: keep-legacy-resource
      type: garbage-collect
      properties:
        keepLegacyResource: true
```

``` shell
kubectl apply -f app1.yaml
```

```shell
$ kubectl get app
NAME             COMPONENT          TYPE         PHASE     HEALTHY   STATUS   AGE
first-vela-app   express-server-1   webservice   running   true               9m35s
```

check whether legacy resources are reserved.

```
$ kubectl get deploy
NAME               READY   UP-TO-DATE   AVAILABLE   AGE
express-server     1/1     1            1           10m
express-server-1   1/1     1            1           40s
```

```
$ kubectl get svc
NAME               TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)    AGE
express-server     ClusterIP   10.96.102.249   <none>        8000/TCP   10m
express-server-1   ClusterIP   10.96.146.10    <none>        8000/TCP   46s
```

```
$ kubectl get ingress
NAME               CLASS    HOSTS                 ADDRESS   PORTS   AGE
express-server     <none>   testsvc.example.com             80      10m
express-server-1   <none>   testsvc.example.com             80      50s
```

```
$ kubectl get resourcetrackers.core.oam.dev
NAME                        AGE
first-vela-app-default      12m
first-vela-app-v1-default   12m
first-vela-app-v2-default   2m56s
```

3. delete the app

```
$ kubectl delete app first-vela-app
```
