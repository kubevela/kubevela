---
title: 聚合健康探针
---

`HealthyScope` 允许您为同一应用程序中的所有组件定义一个聚合的健康探测器。

1. 创建健康范围实例。
```yaml
apiVersion: core.oam.dev/v1alpha2
kind: HealthScope
metadata:
  name: health-check
  namespace: default
spec:
  probe-interval: 60
  workloadRefs:
  - apiVersion: apps/v1
    kind: Deployment
    name: express-server
```
2. 创建落入此运行状况范围内的应用程序。
```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: vela-app
spec:
  components:
    - name: express-server
      type: webservice
      properties:
        image: crccheck/hello-world
        port: 8080 # change port
        cpu: 0.5 # add requests cpu units
      scopes:
        healthscopes.core.oam.dev: health-check
```
3. 检查聚合健康探针的引用（`status.service.scopes`）。
```shell
kubectl get app vela-app -o yaml
```
```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: vela-app
...
status:
...
  services:
   - healthy: true
     name: express-server
     scopes:
       - apiVersion: core.oam.dev/v1alpha2
         kind: HealthScope
         name: health-check
```
4. 检查健康范围详细信息。
```shell
kubectl get healthscope health-check -o yaml
```
```yaml
apiVersion: core.oam.dev/v1alpha2
kind: HealthScope
metadata:
  name: health-check
...
spec:
  probe-interval: 60
  workloadRefs:
    - apiVersion: apps/v1
      kind: Deployment
      name: express-server
status:
  healthConditions:
    - componentName: express-server
      diagnosis: 'Ready:1/1 '
      healthStatus: HEALTHY
      targetWorkload:
        apiVersion: apps/v1
        kind: Deployment
        name: express-server
  scopeHealthCondition:
    healthStatus: HEALTHY
    healthyWorkloads: 1
    total: 1
```

它显示了此应用程序中所有组件的汇总运行状况。
