---
title:  Application CRD
---

本部分将逐步介绍如何使用 `Application` 对象来定义你的应用，并以声明式的方式进行相应的操作。

## 示例

下面的示例应用声明了一个具有 *Worker* 工作负载类型的 `backend` 组件和具有 *Web Service* 工作负载类型的 `frontend` 组件。

此外，`frontend`组件声明了具有 `sidecar` 和 `autoscaler` 的 `trait` 运维能力，这意味着工作负载将自动注入 `fluentd` 的sidecar，并可以根据CPU使用情况触发1-10个副本进行扩展。

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: website
spec:
  components:
    - name: backend
      type: worker
      properties:
        image: busybox
        cmd:
          - sleep
          - '1000'
    - name: frontend
      type: webservice
      properties:
        image: nginx
      traits:
        - type: autoscaler
          properties:
            min: 1
            max: 10
            cpuPercent: 60
        - type: sidecar
          properties:
            name: "sidecar-test"
            image: "fluentd"
```

### 部署应用

部署上述的 application yaml文件, 然后应用启动

```shell
$ kubectl get application -o yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
 name: website
....
status:
  components:
  - apiVersion: core.oam.dev/v1alpha2
    kind: Component
    name: backend
  - apiVersion: core.oam.dev/v1alpha2
    kind: Component
    name: frontend
....
  status: running

```

你可以看到一个命名为 `frontend` 并带有被注入的容器 `fluentd` 的 Deployment 正在运行。

```shell
$ kubectl get deploy frontend
NAME       READY   UP-TO-DATE   AVAILABLE   AGE
frontend   1/1     1            1           100m
```

另一个命名为 `backend` 的 Deployment 也在运行。

```shell
$ kubectl get deploy backend
NAME      READY   UP-TO-DATE   AVAILABLE   AGE
backend   1/1     1            1           100m
```

同样被 `autoscaler` trait 创建出来的还有一个 HPA 。

```shell
$ kubectl get HorizontalPodAutoscaler frontend
NAME       REFERENCE             TARGETS         MINPODS   MAXPODS   REPLICAS   AGE
frontend   Deployment/frontend   <unknown>/50%   1         10        1          101m
```


## 背后的原理

在上面的示例中, `type: worker` 指的是该组件的字段内容（即下面的 `properties` 字段中的内容）将遵从名为 `worker` 的 `ComponentDefinition` 对象中的规范定义，如下所示：

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: worker
  annotations:
    definition.oam.dev/description: "Describes long-running, scalable, containerized services that running at backend. They do NOT have network endpoint to receive external network traffic."
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  schematic:
    cue:
      template: |
        output: {
          apiVersion: "apps/v1"
          kind:       "Deployment"
          spec: {
            selector: matchLabels: {
              "app.oam.dev/component": context.name
            }
            template: {
              metadata: labels: {
                "app.oam.dev/component": context.name
              }
              spec: {
                containers: [{
                  name:  context.name
                  image: parameter.image

                  if parameter["cmd"] != _|_ {
                    command: parameter.cmd
                  }
                }]
              }
            }
          }
        }
        parameter: {
          image: string
          cmd?: [...string]
        }
```

因此，`backend` 的 `properties` 部分仅支持两个参数：`image` 和 `cmd`。这是由定义的 `.spec.template` 字段中的 `parameter` 列表执行的。

类似的可扩展抽象机制也同样适用于 traits(运维能力)。
例如，`frontend` 中的 `type：autoscaler` 指的是组件对应的 trait 的字段规范（即 trait 的 `properties` 部分）
将由名为 `autoscaler` 的 `TraitDefinition` 对象执行，如下所示：

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "configure k8s HPA for Deployment"
  name: hpa
spec:
  appliesToWorkloads:
    - webservice
    - worker
  schematic:
    cue:
      template: |
        outputs: hpa: {
          apiVersion: "autoscaling/v2beta2"
          kind:       "HorizontalPodAutoscaler"
          metadata: name: context.name
          spec: {
            scaleTargetRef: {
              apiVersion: "apps/v1"
              kind:       "Deployment"
              name:       context.name
            }
            minReplicas: parameter.min
            maxReplicas: parameter.max
            metrics: [{
              type: "Resource"
              resource: {
                name: "cpu"
                target: {
                  type:               "Utilization"
                  averageUtilization: parameter.cpuUtil
                }
              }
            }]
          }
        }
        parameter: {
          min:     *1 | int
          max:     *10 | int
          cpuUtil: *50 | int
        }
```

应用同样有一个`sidecar`的运维能力

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "add sidecar to the app"
  name: sidecar
spec:
  appliesToWorkloads:
    - webservice
    - worker
  schematic:
    cue:
      template: |-
        patch: {
           // +patchKey=name
           spec: template: spec: containers: [parameter]
        }
        parameter: {
           name: string
           image: string
           command?: [...string]
        }
```

在业务用户使用之前，我们认为所有用于定义的对象（Definition Object）都已经由平台团队声明并安装完毕了。所以，业务用户将需要专注于应用（`Application`）本身。

请注意，KubeVela 的终端用户（业务研发）不需要了解定义对象，他们只需要学习如何使用平台已经安装的能力，这些能力通常还可以被可视化的表单展示出来（或者通过 JSON schema 对接其他方式）。请从[由定义生成前端表单](/docs/platform-engineers/openapi-v3-json-schema)部分的文档了解如何实现。

### 惯例和"标准协议"

在应用（`Application` 资源）部署到 Kubernetes 集群后，KubeVela 运行时将遵循以下 “标准协议”和惯例来生成和管理底层资源实例。


| Label  | 描述 |
| :--: | :---------: | 
|`workload.oam.dev/type=<component definition name>` | 其对应 `ComponentDefinition` 的名称 |
|`trait.oam.dev/type=<trait definition name>` | 其对应 `TraitDefinition` 的名称 | 
|`app.oam.dev/name=<app name>` | 它所属的应用的名称 |
|`app.oam.dev/component=<component name>` | 它所属的组件的名称 |
|`trait.oam.dev/resource=<name of trait resource instance>` | 运维能力资源实例的名称 |
|`app.oam.dev/appRevision=<name of app revision>` | 它所属的应用revision的名称 |
