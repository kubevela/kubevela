---
title:  Attach Traits
---

All traits in the KubeVela system works well with the Raw K8s Object Template based Component. 

In this sample, we will attach two traits,
[scaler](https://github.com/oam-dev/kubevela/blob/master/charts/vela-core/templates/defwithtemplate/manualscale.yaml)
and
[virtualgroup](https://github.com/oam-dev/kubevela/blob/master/docs/examples/kube-module/virtual-group-td.yaml) to a component

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: myapp
  namespace: default
spec:
  components:
    - name: mycomp
      type: kube-worker
      properties: 
        image: nginx:1.14.0
      traits:
        - type: scaler
          properties:
            replicas: 2
        - type: virtualgroup
          properties:
            group: "my-group1"
            type: "cluster"
```

## Verify

Deploy the application and verify traits work.

Check the `scaler` trait.
```shell
$ kubectl get manualscalertrait
NAME                            AGE
demo-podinfo-scaler-3x1sfcd34   2m
```
```shell
$ kubectl get deployment mycomp -o json | jq .spec.replicas
2
```

Check the `virtualgroup` trait.
```shell
$ kubectl get deployment mycomp -o json | jq .spec.template.metadata.labels
{
  "app.cluster.virtual.group": "my-group1",
  "app.kubernetes.io/name": "myapp"
}
```

## Update an Application

After the application is deployed and workloads/traits are created successfully,
you can update the application, and corresponding changes will be applied to the
workload.

Let's make several changes on the configuration of the sample application.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: myapp
  namespace: default
spec:
  components:
    - name: mycomp
      type: kube-worker
      properties: 
        image: nginx:1.14.1 # 1.14.0 => 1.14.1
      traits:
        - type: scaler
          properties:
            replicas: 4 # 2 => 4
        - type: virtualgroup
          properties:
            group: "my-group2" # my-group1 => my-group2
            type: "cluster"
```

Apply the new configuration and check the results after several seconds.

> After updating, the workload instance name will be updated from `mycomp-v1` to `mycomp-v2`.

Check the new property value.
```shell
$ kubectl get deployment mycomp -o json | jq '.spec.template.spec.containers[0].image'
"nginx:1.14.1"
```

Check the `scaler` trait.
```shell
$ kubectl get deployment mycomp -o json | jq .spec.replicas
4
```

Check the `virtualgroup` trait.
```shell
$ kubectl get deployment mycomp -o json | jq .spec.template.metadata.labels
{
  "app.cluster.virtual.group": "my-group2",
  "app.kubernetes.io/name": "myapp"
}
```
