---
title:  高级功能
---

作为数据配置语言，CUE 对于自定义结构体支持一些黑魔法。

## 循环渲染多个资源

你可以在 `outputs` 定义 for 循环。

> ⚠️注意，本示例中 `parameter` 必须是字典类型。

如下所示，该示例将展示如何在 trait 中渲染多个 Kubernetes Services：

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  name: expose
spec:
  schematic:
    cue:
      template: |
        parameter: {
          http: [string]: int
        }

        outputs: {
          for k, v in parameter.http {
            "\(k)": {
              apiVersion: "v1"
              kind:       "Service"
              spec: {
                selector:
                  app: context.name
                ports: [{
                  port:       v
                  targetPort: v
                }]
              }
            }
          }
        }
```

上面 trait 对象可以在以下 Application 被使用：

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: testapp
spec:
  components:
    - name: express-server
      type: webservice
      properties:
        ...
      traits:
        - type: expose
          properties:
            http:
              myservice1: 8080
              myservice2: 8081
```

## Trait Definition 中请求 HTTP 接口

Trait Definition 中可以发送 HTTP 请求并借助字段 `processing` 将响应结果用于渲染资源。

你可以在 `processing.http` 字段下定义 HTTP 请求所需的字段，包括：`method`， `url`， `body`， `header` 和 `trailer` ，响应将会被存储在 `processing.output` 字段中。

> 此处需要确认目标 HTTP 服务返回数据格式为 **JSON**。

随后你可以在 `patch` 或者 `output/outputs` 字段中引用 `processing.output` 自动中的返回数据。

如下所示：

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  name: auth-service
spec:
  schematic:
    cue:
      template: |
        parameter: {
          serviceURL: string
        }

        processing: {
          output: {
            token?: string
          }
          // The target server will return a JSON data with `token` as key.
          http: {
            method: *"GET" | string
            url:    parameter.serviceURL
            request: {
              body?: bytes
              header: {}
              trailer: {}
            }
          }
        }

        patch: {
          data: token: processing.output.token
        }
```

以上示例，该 Trait Definition 将发送请求获取 `token` 信息，并将数据插入到给定到 component 实例中。

## 数据传递

TraitDefinition 可以从给定的 ComponentDefinition 中读取已经被生成的 API 资源（从 `output` and `outputs` 中被渲染）。

>  KubeVela 会确保 ComponentDefinition 会先于 TraitDefinition 被渲染出来。

具体来说，`context.output` 字段中会包含已经被渲染的 workload API 资源（特指 GVK 已经在 ComponentDefinition 中 `spec.workload` 字段定义的资源），同时 `context.outputs.<xx>` 
字段中会包含其他已经被渲染的非 workload API 资源。

下面是数据传递的示例：

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: worker
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
                  ports: [{containerPort: parameter.port}]
                  envFrom: [{
                    configMapRef: name: context.name + "game-config"
                  }]
                  if parameter["cmd"] != _|_ {
                    command: parameter.cmd
                  }
                }]
              }
            }
          }
        }

        outputs: gameconfig: {
          apiVersion: "v1"
          kind:       "ConfigMap"
          metadata: {
            name: context.name + "game-config"
          }
          data: {
            enemies: parameter.enemies
            lives:   parameter.lives
          }
        }

        parameter: {
          // +usage=Which image would you like to use for your service
          // +short=i
          image: string
          // +usage=Commands to run in the container
          cmd?: [...string]
          lives:   string
          enemies: string
          port:    int
        }


---
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  name: ingress
spec:
  schematic:
    cue:
      template: |
        parameter: {
          domain:     string
          path:       string
          exposePort: int
        }
        // trait template can have multiple outputs in one trait
        outputs: service: {
          apiVersion: "v1"
          kind:       "Service"
          spec: {
            selector:
              app: context.name
            ports: [{
              port:       parameter.exposePort
              targetPort: context.output.spec.template.spec.containers[0].ports[0].containerPort
            }]
          }
        }
        outputs: ingress: {
          apiVersion: "networking.k8s.io/v1beta1"
          kind:       "Ingress"
          metadata:
              name: context.name
          labels: config: context.outputs.gameconfig.data.enemies
          spec: {
            rules: [{
              host: parameter.domain
              http: {
                paths: [{
                  path: parameter.path
                  backend: {
                    serviceName: context.name
                    servicePort: parameter.exposePort
                  }
                }]
              }
            }]
          }
        }
```

关于 `worker` `ComponentDefinition` 渲染期间的一些细节：
1. workload，渲染完成的 Kubernetes Deployment 资源将存储在 `context.output` 字段中。
2. 非 workload，其他渲染完成的资源将存储在 `context.outputs.<xx>` 字段中，其中 `<xx>` 在每个 `template.outputs` 字段中名字都是唯一的。

综上，`TraitDefinition` 可以从 `context` 字段读取完成渲染的 API 资源（比如：`context.outputs.gameconfig.data.enemies`）。
