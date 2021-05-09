---
title: Labels and Annotations
---


## List Traits

The `label` and `annotations` traits allows you to append labels and annotations to the component.

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

Deploy this application.

```shell
kubectl apply -f myapp.yaml
```

On runtime cluster, check the workload has been created successfully.

```bash
$ kubectl get deployments
NAME             READY   UP-TO-DATE   AVAILABLE   AGE
express-server   1/1     1            1           15s
```

Check the `labels`.

```bash
$ kubectl get deployments express-server -o jsonpath='{.spec.template.metadata.labels}'
{"app.oam.dev/component":"express-server","release": "stable"}
```

Check the `annotations`.

```bash
$ kubectl get deployments express-server -o jsonpath='{.spec.template.metadata.annotations}'
{"description":"web application"}
```
