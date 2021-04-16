---
title: Labels and Annotations
---

We will introduce how to add labels and annotations to your Application.

## List Traits

```bash
$ kubectl get trait -n vela-system
NAME          APPLIES-TO                DESCRIPTION
annotations   ["webservice","worker"]   Add annotations for your Workload.
cpuscaler     ["webservice","worker"]   configure k8s HPA with CPU metrics for Deployment
ingress       ["webservice","worker"]   Configures K8s ingress and service to enable web traffic for your service. Please use route trait in cap center for advanced usage.
labels        ["webservice","worker"]   Add labels for your Workload.
scaler        ["webservice","worker"]   Configures replicas for your service by patch replicas field.
sidecar       ["webservice","worker"]   inject a sidecar container into your app
```

You can use `label` and `annotations` traits to add labels and annotations for your workload.

## Apply Application

Let's use `label` and `annotations` traits in your Application.

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

Apply this Application.

```shell
kubectl apply -f myapp.yaml
```

Check the workload has been created successfully.

```bash
$ kubectl get deployments
NAME             READY   UP-TO-DATE   AVAILABLE   AGE
express-server   1/1     1            1           15s
```

Check the `labels` trait.

```bash
$ kubectl get deployments express-server -o jsonpath='{.spec.template.metadata.labels}'
{"app.oam.dev/component":"express-server","release": "stable"}
```

Check the `annotations` trait.

```bash
$ kubectl get deployments express-server -o jsonpath='{.spec.template.metadata.annotations}'
{"description":"web application"}
```
