---
title:  Patch Traits
---

**Patch** 是 trait 定义的一种非常常见的模式， 即应用操作员可以修改或者将路径属性设置为组件实例（通常是 workload ）以启用某些操作功能例如 sidecar 或节点相似性规则（这应该在将资源应用于目标集群 **之前** 完成）。

当 component 定义由第三方 component 提供程序（例如，软件发行商）提供时，此模式非常有用，因此应用操作员无权更改其模板。

> 请注意，即使 patch trait 本身是由 CUE 定义的，它也可以修补任何 component，无论其基于什么原理（即 CUE，Helm 和任何其他受支持的方式）。

下面是 `node-affinity` trait 的例子：

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "affinity specify node affinity and toleration"
  name: node-affinity
spec:
  appliesToWorkloads:
    - webservice
    - worker
  podDisruptive: true
  schematic:
    cue:
      template: |
        patch: {
        	spec: template: spec: {
        		if parameter.affinity != _|_ {
        			affinity: nodeAffinity: requiredDuringSchedulingIgnoredDuringExecution: nodeSelectorTerms: [{
        				matchExpressions: [
        					for k, v in parameter.affinity {
        						key:      k
        						operator: "In"
        						values:   v
        					},
        				]}]
        		}
        		if parameter.tolerations != _|_ {
        			tolerations: [
        				for k, v in parameter.tolerations {
        					effect:   "NoSchedule"
        					key:      k
        					operator: "Equal"
        					value:    v
        				}]
        		}
        	}
        }

        parameter: {
        	affinity?: [string]: [...string]
        	tolerations?: [string]: string
        }
```

上面的 patch trait 假定目标组件实例具有 `spec.template.spec.affinity` 字段。
因此，我们需要使用 `applyToWorkloads` 来强制执行该 trait，仅适用于具有此字段的那些 workload 类型。

另外一个重要的字段是  `podDisruptive`，此 patch trait 将修改到 Pod 模板字段，因此对该 trait 的任何字段进行更改都会导致 Pod 重新启动，我们应该增加 `podDisruptive` 并且设置它的值为 true 
以此告诉用户应用此 trait 将导致 Pod 重新启动。

现在，用户可以声明他们想要将节点相似性规则添加到 component 实例，如下所示：

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: Application
metadata:
  name: testapp
spec:
  components:
    - name: express-server
      type: webservice
      properties:
        image: oamdev/testapp:v1
      traits:
        - type: "node-affinity"
          properties:
            affinity:
              server-owner: ["owner1","owner2"]
              resource-pool: ["pool1","pool2","pool3"]
            tolerations:
              resource-pool: "broken-pool1"
              server-owner: "old-owner"
```

### 已知局限性

默认情况下，KubeVela 中 patch trait 使用 CUE `merge` 操作。它具有以下已知约束

- 无法处理冲突。
  - 例如，如果已将 component 实例的值设置为 `replicas=5`，则修改 `replicas` 字段的任何 patch trait 都将失败，也就是你不应在其 component 定义中公开 `replicas` 字段。
- patch 中的数组列表将按照索引顺序合并。它无法处理数组列表成员的重复。但这可以通过下面的另一个功能解决。

### 策略 Patch

`strategy patch` 对修改数组列表很有用。

> 请注意，这不是标准的 CUE 功能，KubeVela 增强了 CUE 在这个场景的能力

使用 `//+patchKey=<key_name>` 注释，两个数组列表的合并逻辑将不遵循 CUE 行为。相反，它将列表视为对象并使用策略合并方法：
 - 如果找到重复的 key，则修改数据将与现有值合并；
 - 如果找不到重复项，则修改将追加到数组列表中。

策略 patch trait 的示例如下所示：
 
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
  podDisruptive: true
  schematic:
    cue:
      template: |
        patch: {
        	// +patchKey=name
        	spec: template: spec: containers: [parameter]
        }
        parameter: {
        	name:  string
        	image: string
        	command?: [...string]
        }
```

在上面的示例中，我们定义了 `patchKey` 为 `name` ，这是容器名称的参数 key 。 在这种情况下，如果 workload 中没有相同名称的容器，它将是一个 sidecar 容器，追加到 `spec.template.spec.containers` 数组列表中。 如果 workload 已经有一个具有与此 `Sidecar` trait 相同名称的容器，则将发生合并操作而不是追加操作（这将导致重复的容器）。

如果 `patch` and `outputs` 都存在于一个 trait 定义中，则将首先处理 `patch` 操作，然后呈现 `outputs` 。

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "expose the app"
  name: expose
spec:
  appliesToWorkloads:
    - webservice
    - worker
  podDisruptive: true
  schematic:
    cue:
      template: |
        patch: {spec: template: metadata: labels: app: context.name}
        outputs: service: {
        	apiVersion: "v1"
        	kind:       "Service"
        	metadata: name: context.name
        	spec: {
        		selector: app: context.name
        		ports: [
        			for k, v in parameter.http {
        				port:       v
        				targetPort: v
        			},
        		]
        	}
        }
        parameter: {
        	http: [string]: int
        }
```

因此，将 Service 附加到给定 component 实例的上述 trait 将首先为 workload 打上相应的标签，然后基于 `outputs` 中的模板呈现服 Service 资源。

## Patch Trait 的更多使用案例

通常，patch trait 非常有用，可以将操作问题与 component 定义分开，下面有更多示例。

### 添加标签

例如，修改 component 实例通用标签（虚拟组）。

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "Add virtual group labels"
  name: virtualgroup
spec:
  appliesToWorkloads:
    - webservice
    - worker
  podDisruptive: true
  schematic:
    cue:
      template: |
        patch: {
        	spec: template: {
        		metadata: labels: {
        			if parameter.scope == "namespace" {
        				"app.namespace.virtual.group": parameter.group
        			}
        			if parameter.scope == "cluster" {
        				"app.cluster.virtual.group": parameter.group
        			}
        		}
        	}
        }
        parameter: {
        	group: *"default" | string
        	scope:  *"namespace" | string
        }
```

然后可以像这样使用：

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
spec:
  ...
      traits:
      - type: virtualgroup
        properties:
          group: "my-group1"
          scope: "cluster"
```

### 添加注释

与常见标签类似，你也可以使用注释来修补 component 实例。注释值应为 JSON 字符串。

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "Specify auto scale by annotation"
  name: kautoscale
spec:
  appliesToWorkloads:
    - webservice
    - worker
  podDisruptive: false
  schematic:
    cue:
      template: |
        import "encoding/json"

        patch: {
        	metadata: annotations: {
        		"my.custom.autoscale.annotation": json.Marshal({
        			"minReplicas": parameter.min
        			"maxReplicas": parameter.max
        		})
        	}
        }
        parameter: {
        	min: *1 | int
        	max: *3 | int
        }
```

### 添加 Pod 环境变量

将系统环境注入 Pod 也是非常常见的例子。

> 这种情况取决于策略合并修改，因此不要忘记添加 `+patchKey=name` ，如下所示：

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "add env into your pods"
  name: env
spec:
  appliesToWorkloads:
    - webservice
    - worker
  podDisruptive: true
  schematic:
    cue:
      template: |
        patch: {
        	spec: template: spec: {
        		// +patchKey=name
        		containers: [{
        			name: context.name
        			// +patchKey=name
        			env: [
        				for k, v in parameter.env {
        					name:  k
        					value: v
        				},
        			]
        		}]
        	}
        }

        parameter: {
        	env: [string]: string
        }
```

### 基于外部身份验证服务注入 `ServiceAccount`

在此示例中，从身份验证服务动态请求了服务帐户并将其修补到该服务中。

此示例将 UID 令牌放在 HTTP 头中，但如果愿意，也可以使用请求体。

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "dynamically specify service account"
  name: service-account
spec:
  appliesToWorkloads:
    - webservice
    - worker
  podDisruptive: true
  schematic:
    cue:
      template: |
        processing: {
        	output: {
        		credentials?: string
        	}
        	http: {
        		method: *"GET" | string
        		url:    parameter.serviceURL
        		request: {
        			header: {
        				"authorization.token": parameter.uidtoken
        			}
        		}
        	}
        }
        patch: {
        	spec: template: spec: serviceAccountName: processing.output.credentials
        }

        parameter: {
        	uidtoken:   string
        	serviceURL: string
        }
```

`processing.http` 部分是高级功能，允许 trait 定义在渲染资源期间发送 HTTP 请求。有关更多详细信息，请参考[特质定义中的执行HTTP请求](#Processing-Trait) 部分。

### 添加 `InitContainer`

[`InitContainer`](https://kubernetes.io/docs/tasks/configure-pod-container/configure-pod-initialization/#create-a-pod-that-has-an-init-container) 对在 image 中预定义操作并在应用程序容器之前运行它很有用。

下面是一个例子：

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "add an init container and use shared volume with pod"
  name: init-container
spec:
  appliesToWorkloads:
    - webservice
    - worker
  podDisruptive: true
  schematic:
    cue:
      template: |
        patch: {
        	spec: template: spec: {
        		// +patchKey=name
        		containers: [{
        			name: context.name
        			// +patchKey=name
        			volumeMounts: [{
        				name:      parameter.mountName
        				mountPath: parameter.appMountPath
        			}]
        		}]
        		initContainers: [{
        			name:  parameter.name
        			image: parameter.image
        			if parameter.command != _|_ {
        				command: parameter.command
        			}

        			// +patchKey=name
        			volumeMounts: [{
        				name:      parameter.mountName
        				mountPath: parameter.initMountPath
        			}]
        		}]
        		// +patchKey=name
        		volumes: [{
        			name: parameter.mountName
        			emptyDir: {}
        		}]
        	}
        }

        parameter: {
        	name:  string
        	image: string
        	command?: [...string]
        	mountName:     *"workdir" | string
        	appMountPath:  string
        	initMountPath: string
        }
```

用法可以是：

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
        image: oamdev/testapp:v1
      traits:
        - type: "init-container"
          properties:
            name:  "install-container"
            image: "busybox"
            command:
            - wget
            - "-O"
            - "/work-dir/index.html"
            - http://info.cern.ch
            mountName: "workdir"
            appMountPath:  "/usr/share/nginx/html"
            initMountPath: "/work-dir"
```
