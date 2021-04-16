---
title: Scale
---

In the [Deploy Application](../application) section, we use `cpuscaler` trait as an auto-scaler for the sample application. 

## Manuel Scale

You can use scale your application manually by using `scaler` trait.

```shell
$ kubectl vela show scaler 
# Properties
+----------+--------------------------------+------+----------+---------+
|   NAME   |          DESCRIPTION           | TYPE | REQUIRED | DEFAULT |
+----------+--------------------------------+------+----------+---------+
| replicas | Specify replicas of workload   | int  | true     |       1 |
+----------+--------------------------------+------+----------+---------+
```

Deploy the application.

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

Change and Apply the sample application:

```shell
$ kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/docs/examples/enduser/sample-manual.yaml
application.core.oam.dev/website configured
```

After a while, you can see the underlying deployment of `frontend` component has two replicas now.

```shell
$ kubectl get deploy -l app.oam.dev/name=website
NAME       READY   UP-TO-DATE   AVAILABLE   AGE
backend    1/1     1            1           19h
frontend   2/2     2            2           19h
```

To scale up or scale down, you can just modify the `replicas` field of `scaler` trait and apply the application again.