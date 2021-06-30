---
title:  添加 Traits
---

通过 Component，KubeVela 中的所有 traits 都可以兼容原生的 K8s 对象模板。

在这个例子中，我们会添加两个 traits 到 component 中。分别是：[scaler](https://github.com/oam-dev/kubevela/blob/master/charts/vela-core/templates/defwithtemplate/manualscale.yaml) 和 [virtualgroup](https://github.com/oam-dev/kubevela/blob/master/docs/examples/kube-module/virtual-group-td.yaml)

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

## 验证

部署应用，验证 traits 正常运行

检查 `scaler` trait。

```shell
$ kubectl get manualscalertrait
NAME                            AGE
demo-podinfo-scaler-3x1sfcd34   2m
```
```shell
$ kubectl get deployment mycomp -o json | jq .spec.replicas
2
```

检查 `virtualgroup` trait。

```shell
$ kubectl get deployment mycomp -o json | jq .spec.template.metadata.labels
{
  "app.cluster.virtual.group": "my-group1",
  "app.kubernetes.io/name": "myapp"
}
```

## 更新应用

在应用部署完后（同时 workloads/trait 成功地创建），你可以执行更新应用的操作，并且更新的内容会被应用到 workload 上。

下面来演示修改上面部署的应用的几个配置

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

应用上面的配置，几秒后检查配置。

> 更新配置后，workload 实例的名称会被修改成 `mycomp-v2`

检查新的属性值

```shell
$ kubectl get deployment mycomp -o json | jq '.spec.template.spec.containers[0].image'
"nginx:1.14.1"
```

检查 `scaler` trait。

```shell
$ kubectl get deployment mycomp -o json | jq .spec.replicas
4
```

检查 `virtualgroup` trait

```shell
$ kubectl get deployment mycomp -o json | jq .spec.template.metadata.labels
{
  "app.cluster.virtual.group": "my-group2",
  "app.kubernetes.io/name": "myapp"
}
```
