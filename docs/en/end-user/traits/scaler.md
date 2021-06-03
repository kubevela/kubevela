---
title: Manual Scaling
---

The `scaler` trait allows you to scale your component instance manually.

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

Declare an application with scaler trait.

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

Apply the sample application:

```shell
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/docs/examples/enduser/sample-manual.yaml
```
```console
application.core.oam.dev/website configured
```

In runtime cluster, you can see the underlying deployment of `frontend` component has 2 replicas now.

```shell
kubectl get deploy -l app.oam.dev/name=website
```
```console
NAME       READY   UP-TO-DATE   AVAILABLE   AGE
backend    1/1     1            1           19h
frontend   2/2     2            2           19h
```

To scale up or scale down, you just need to modify the `replicas` field of `scaler` trait and re-apply the YAML.
