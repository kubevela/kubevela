---
title:  如何定义
---

在本节中，我们将介绍如何定义 Trait。

## 简单 Trait

可以通过简单地参考现有的 Kubernetes API 资源来定义 KubeVela 中的 Trait。

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  name: ingress
spec:
  definitionRef:
    name: ingresses.networking.k8s.io
```
让我们将此 Trait 附加到 `Application` 中的 Component 实例：

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
        cmd:
          - node
          - server.js
        image: oamdev/testapp:v1
        port: 8080
      traits:
        - type: ingress
          properties:
            rules:
            - http:
                paths:
                - path: /testpath
                  pathType: Prefix
                  backend:
                    service:
                      name: test
                      port:
                        number: 80
```

注意在这个例子中，所引用资源的 `spec` 中的所有字段都将向最终用户公开，并且不允许将任何元数据（例如 `annotations` 等）设置为 Trait 的属性。 因此，当你希望将自己的 CRD 和控制器作为 Trait 时，通常使用此方法，并且它不依赖 `annotations` 等作为调整手段。

## 使用 CUE 来构建 Trait

也推荐使用 CUE 的方式来定义 Trait。在这个例子中，它带有抽象，你可以完全灵活地根据需要来模板化任何资源和字段。请注意，KubeVela 要求所有 Trait 必须在 CUE 模板的 `outputs` 部分（而非 `output` ）中定义，格式如下： 

```cue
outputs: <unique-name>: 
  <full template data>
```

以下是 `ingress` 的 Trait 示例。

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  name: ingress
spec:
  podDisruptive: false
  schematic:
    cue:
      template: |
        parameter: {
        	domain: string
        	http: [string]: int
        }

        // trait template can have multiple outputs in one trait
        outputs: service: {
        	apiVersion: "v1"
        	kind:       "Service"
        	spec: {
        		selector:
        			app: context.name
        		ports: [
        			for k, v in parameter.http {
        				port:       v
        				targetPort: v
        			},
        		]
        	}
        }

        outputs: ingress: {
        	apiVersion: "networking.k8s.io/v1beta1"
        	kind:       "Ingress"
        	metadata:
        		name: context.name
        	spec: {
        		rules: [{
        			host: parameter.domain
        			http: {
        				paths: [
        					for k, v in parameter.http {
        						path: k
        						backend: {
        							serviceName: context.name
        							servicePort: v
        						}
        					},
        				]
        			}
        		}]
        	}
        }
```

让我们将此 Trait 附加到`Application`中的 Component 实例中：

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
        cmd:
          - node
          - server.js
        image: oamdev/testapp:v1
        port: 8080
      traits:
        - type: ingress
          properties:
            domain: test.my.domain
            http:
              "/api": 8080
```

基于 CUE 的 Trait 定义还可以支持许多其他高级方案，例如修补和数据传递。 在接下来的文档中将对它们进行详细说明。