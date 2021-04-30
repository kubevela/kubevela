---
title: Define Application Health Probe
---

In this documentation, we will show how to define health probe for application

## Set Health Check Rule

Basically you can set application `spec.status.healthPolicy` field to specifying health check rule for application. [reference](../cue/status)

## Advanced Health Probe

By using HealthyScope you can check all pods of workload weather are healthy.

1.Create health scope by apply this yaml
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
2. Create an application with the health scope
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
3. Check app status, will see health scope in `status.service.scopes`
```shell
$ kubectl get app vela-app -o yaml
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
4.Check health scope status
```shell
$ kubectl get healthscope health-check -o yaml
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
