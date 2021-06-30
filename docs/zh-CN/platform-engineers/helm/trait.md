---
标题:  添加 Trait 特性
---

KubeVela 中的 Trait 特性可以从基于Helm的组件无缝添加.

在以下应用实例中，我们将基于 Helm 组件添加两个 Trait 特性 [scaler](https://github.com/oam-dev/kubevela/blob/master/charts/vela-core/templates/defwithtemplate/manualscale.yaml) 和 [virtualgroup](https://github.com/oam-dev/kubevela/blob/master/docs/examples/helm-module/virtual-group-td.yaml).





```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: myapp
  namespace: default
spec:
  components:
    - name: demo-podinfo 
      type: webapp-chart
      properties: 
        image:
          tag: "5.1.2"
      traits:
        - type: scaler
          properties:
            replicas: 4
        - type: virtualgroup
          properties:
            group: "my-group1"
            type: "cluster"
```

> 注意: 当我们使用基于 Helm 的 Trait 特性时, *请确认在你 Helm 图标中的目标负载严格按照 qualified-full-name convention in Helm 的命名方式.* [以此表为例](https://github.com/captainroy-hy/podinfo/blob/c2b9603036f1f033ec2534ca0edee8eff8f5b335/charts/podinfo/templates/deployment.yaml#L4), 
> 负载名为[版本名和图表名](https://github.com/captainroy-hy/podinfo/blob/c2b9603036f1f033ec2534ca0edee8eff8f5b335/charts/podinfo/templates/_helpers.tpl#L13).

> 这是因为 KubeVela 依赖命名去发现负载,否则将不能把 Trait 特性赋予负载. KubeVela 将会基于你的应用和组件自动生成版本名, 所以你需要保证不能超出你的 Helm 图表中命名模版格式.

## 验证特性工作正确

> 因为应用内部的调整生效需要几秒钟时间.

检查缩放组 `scaler` 特性生效.
```shell
$ kubectl get manualscalertrait
NAME                            AGE
demo-podinfo-scaler-d8f78c6fc   13m
```
```shell
$ kubectl get deployment myapp-demo-podinfo -o json | jq .spec.replicas
4
```

检查虚拟组 `virtualgroup` 特性.
```shell
$ kubectl get deployment myapp-demo-podinfo -o json | jq .spec.template.metadata.labels
{
  "app.cluster.virtual.group": "my-group1",
  "app.kubernetes.io/name": "myapp-demo-podinfo"
}
```

## 更新应用

当应用已被部署且 workload 负载/ Trait 特性都被顺利建立时,
你可以更新应用, 变化会被负载实例所响应.

让我们对实例应用的配置做几个改动.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: myapp
  namespace: default
spec:
  components:
    - name: demo-podinfo 
      type: webapp-chart
      properties: 
        image:
          tag: "5.1.3" # 5.1.2 => 5.1.3 
      traits:
        - type: scaler
          properties:
            replicas: 2 # 4 => 2
        - type: virtualgroup
          properties:
            group: "my-group2" # my-group1 => my-group2
            type: "cluster"
```

在几分钟后应用新配置并检查效果.

检查从应用属性 `properties` 的新值 (`image.tag = 5.1.3`) 已被赋予图表.
```shell
$ kubectl get deployment myapp-demo-podinfo -o json | jq '.spec.template.spec.containers[0].image'
"ghcr.io/stefanprodan/podinfo:5.1.3"
```
实际上, Helm 更新了版本号 (revision 1 => 2).
```shell
$ helm ls -A
NAME              	NAMESPACE	REVISION	UPDATED                                	STATUS  	CHART        	APP VERSION
myapp-demo-podinfo	default  	2       	2021-03-15 08:52:00.037690148 +0000 UTC	deployed	podinfo-5.1.4	5.1.4
```

检查 `scaler` 的特性.
```shell
$ kubectl get deployment myapp-demo-podinfo -o json | jq .spec.replicas
2
```

检查 `virtualgroup` 的特性.
```shell
$ kubectl get deployment myapp-demo-podinfo -o json | jq .spec.template.metadata.labels
{
  "app.cluster.virtual.group": "my-group2",
  "app.kubernetes.io/name": "myapp-demo-podinfo"
}
```

## 去除 Trait 特性

让我们试试从应用中去除特性.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: Application
metadata:
  name: myapp
  namespace: default
spec:
  components:
    - name: demo-podinfo 
      type: webapp-chart 
      settings: 
        image:
          tag: "5.1.3"
      traits:
        # - name: scaler
        #   properties:
        #    replicas: 2 
        - name: virtualgroup
          properties:
            group: "my-group2"
            type: "cluster"
```

更新应用实例并检查 `manualscalertrait` 已被删除.
```shell
$ kubectl get manualscalertrait
No resources found
```

