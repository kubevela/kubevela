---
title:  调试， 测试 以及 Dry-run
---

基于具有强大灵活抽象能力的 CUE 定义的模版来说，调试、测试以及 dry-run 非常重要。本教程将逐步介绍如何进行调试。

## 前提

请确保你的环境已经安装以下 CLI ：
* [`cue` >=v0.2.2](https://cuelang.org/docs/install/)

## 定义 Definition 和 Template

我们建议将 `Definition Object` 定义拆分为两个部分：CRD 部分和 CUE 模版部分。前面的拆分会帮忙我们对 CUE 模版进行调试、测试以及 dry-run 操作。

我们将 CRD 部分保存到 `def.yaml` 文件。

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: microservice
  annotations:
    definition.oam.dev/description: "Describes a microservice combo Deployment with Service."
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  schematic:
    cue:
      template: |
```

同时将 CUE 模版部分保存到 `def.cue` 文件，随后我们可以使用 CUE 命令行（`cue fmt` / `cue vet`）格式化和校验 CUE 文件。

```
output: {
	// Deployment
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: {
		name:      context.name
		namespace: "default"
	}
	spec: {
		selector: matchLabels: {
			"app": context.name
		}
		template: {
			metadata: {
				labels: {
					"app":     context.name
					"version": parameter.version
				}
			}
			spec: {
				serviceAccountName:            "default"
				terminationGracePeriodSeconds: parameter.podShutdownGraceSeconds
				containers: [{
					name:  context.name
					image: parameter.image
					ports: [{
						if parameter.containerPort != _|_ {
							containerPort: parameter.containerPort
						}
						if parameter.containerPort == _|_ {
							containerPort: parameter.servicePort
						}
					}]
					if parameter.env != _|_ {
						env: [
							for k, v in parameter.env {
								name:  k
								value: v
							},
						]
					}
					resources: {
						requests: {
							if parameter.cpu != _|_ {
								cpu: parameter.cpu
							}
							if parameter.memory != _|_ {
								memory: parameter.memory
							}
						}
					}
				}]
			}
		}
	}
}
// Service
outputs: service: {
	apiVersion: "v1"
	kind:       "Service"
	metadata: {
		name: context.name
		labels: {
			"app": context.name
		}
	}
	spec: {
		type: "ClusterIP"
		selector: {
			"app": context.name
		}
		ports: [{
			port: parameter.servicePort
			if parameter.containerPort != _|_ {
				targetPort: parameter.containerPort
			}
			if parameter.containerPort == _|_ {
				targetPort: parameter.servicePort
			}
		}]
	}
}
parameter: {
	version:        *"v1" | string
	image:          string
	servicePort:    int
	containerPort?: int
	// +usage=Optional duration in seconds the pod needs to terminate gracefully
	podShutdownGraceSeconds: *30 | int
	env: [string]: string
	cpu?:    string
	memory?: string
}
```

以上操作完成之后，使用该脚本 [`hack/vela-templates/mergedef.sh`](https://github.com/oam-dev/kubevela/blob/master/hack/vela-templates/mergedef.sh) 将 `def.yaml` 和 `def.cue` 合并到完整的 Definition 对象中。

```shell
$ ./hack/vela-templates/mergedef.sh def.yaml def.cue > microservice-def.yaml
```

## 调试 CUE 模版

### 使用 `cue vet` 进行校验

```shell
$ cue vet def.cue
output.metadata.name: reference "context" not found:
    ./def.cue:6:14
output.spec.selector.matchLabels.app: reference "context" not found:
    ./def.cue:11:11
output.spec.template.metadata.labels.app: reference "context" not found:
    ./def.cue:16:17
output.spec.template.spec.containers.name: reference "context" not found:
    ./def.cue:24:13
outputs.service.metadata.name: reference "context" not found:
    ./def.cue:62:9
outputs.service.metadata.labels.app: reference "context" not found:
    ./def.cue:64:11
outputs.service.spec.selector.app: reference "context" not found:
    ./def.cue:70:11
```

常见错误 `reference "context" not found` 主要发生在 [`context`](./cue/component#cue-context)，该部分是仅在 KubeVela 控制器中存在的运行时信息。我们可以在 `def.cue` 中模拟 `context` ，从而对 CUE 模版进行 end-to-end 的校验操作。

> 注意，完成校验测试之后需要清除所有模拟数据。

```CUE
... // existing template data
context: {
    name: string
}
```

随后执行命令：

```shell
$ cue vet def.cue
some instances are incomplete; use the -c flag to show errors or suppress this message
```

该错误 `reference "context" not found` 已经被解决，但是 `cue vet` 仅对数据类型进行校验，这还不能证明模版逻辑是准确对。因此，我们需要使用 `cue vet -c` 完成最终校验：

```shell
$ cue vet def.cue -c
context.name: incomplete value string
output.metadata.name: incomplete value string
output.spec.selector.matchLabels.app: incomplete value string
output.spec.template.metadata.labels.app: incomplete value string
output.spec.template.spec.containers.0.image: incomplete value string
output.spec.template.spec.containers.0.name: incomplete value string
output.spec.template.spec.containers.0.ports.0.containerPort: incomplete value int
outputs.service.metadata.labels.app: incomplete value string
outputs.service.metadata.name: incomplete value string
outputs.service.spec.ports.0.port: incomplete value int
outputs.service.spec.ports.0.targetPort: incomplete value int
outputs.service.spec.selector.app: incomplete value string
parameter.image: incomplete value string
parameter.servicePort: incomplete value int
```

此时，命令行抛出运行时数据不完整的异常（主要因为 `context` 和 `parameter` 字段字段中还有设置值），现在我们填充更多的模拟数据到 `def.cue` 文件：

```CUE
context: {
	name: "test-app"
}
parameter: {
	version:       "v2"
	image:         "image-address"
	servicePort:   80
	containerPort: 8000
	env: {"PORT": "8000"}
	cpu:    "500m"
	memory: "128Mi"
}
```

此时，执行以下命令行没有抛出异常，说明逻辑校验通过：

```shell
cue vet def.cue -c
```

#### 使用 `cue export` 校验已渲染的资源

该命令行 `cue export` 将会渲染结果以 YAML 格式导出：

```shell
$ cue export -e output def.cue --out yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-app
  namespace: default
spec:
  selector:
    matchLabels:
      app: test-app
  template:
    metadata:
      labels:
        app: test-app
        version: v2
    spec:
      serviceAccountName: default
      terminationGracePeriodSeconds: 30
      containers:
        - name: test-app
          image: image-address
```

```shell
$ cue export -e outputs.service def.cue --out yaml
apiVersion: v1
kind: Service
metadata:
  name: test-app
  labels:
    app: test-app
spec:
  selector:
    app: test-app
  type: ClusterIP
```

### 测试使用 `Kube` 包的 CUE 模版

KubeVela 将所有内置 Kubernetes API 资源以及 CRD 自动生成为内部 CUE 包。
你可以将它们导入CUE模板中，以简化模板以及帮助你进行验证。

目前有两种方式来导入内部 `kube` 包。

1. 以固定方式导入： `kube/<apiVersion>` ，这样我们就可以直接引用 `Kind` 对应的结构体。
    ```cue
    import (
     apps "kube/apps/v1"
     corev1 "kube/v1"
    )
    // output is validated by Deployment.
    output: apps.#Deployment
    outputs: service: corev1.#Service
   ```
   这是比较好记易用的方式，主要因为它与 Kubernetes Object 的用法一致，只需要在 `apiVersion` 之前添加前缀 `kube/`。
   当然，这个方式仅在 KubeVela 中被支持，所以你只能通过该方法 [`vela system dry-run`](#dry-run-the-application) 进行调试和测试。
   
2. 以第三方包的方式导入。 
	你可以运行 `vela system cue-packages` 获取所有内置 `kube` 包，通过这个方式可以了解当前支持的 `third-party packages`。

    ```shell
    $ vela system cue-packages
    DEFINITION-NAME                	IMPORT-PATH                         	 USAGE
    #Deployment                    	k8s.io/apps/v1                      	Kube Object for apps/v1.Deployment
    #Service                       	k8s.io/core/v1                      	Kube Object for v1.Service
    #Secret                        	k8s.io/core/v1                      	Kube Object for v1.Secret
    #Node                          	k8s.io/core/v1                      	Kube Object for v1.Node
    #PersistentVolume              	k8s.io/core/v1                      	Kube Object for v1.PersistentVolume
    #Endpoints                     	k8s.io/core/v1                      	Kube Object for v1.Endpoints
    #Pod                           	k8s.io/core/v1                      	Kube Object for v1.Pod
    ```
   其实，这些都是内置包，只是你可以像 `third-party packages` 一样使用 `import-path` 导入这些包。
   当前方式你可以使用 `cue` 命令行进行调试。
   

#### 使用 `Kube` 包的 CUE 模版调试流程

此部分主要介绍使用 `cue` 命令行对  CUE 模版调试和测试的流程，并且可以在 KubeVela中使用 **完全相同的 CUE 模版**。

1. 创建目录，初始化 CUE 模块

```shell
mkdir cue-debug && cd cue-debug/
cue mod init oam.dev
go mod init oam.dev
touch def.cue
```

2. 使用 `cue` 命令行下载 `third-party packages`

其实在 KubeVela 中并不需要下载这些包，因为它们已经被从 Kubernetes API 自动生成。
但是在本地测试环境，我们需要使用 `cue get go`  来获取 Go 包并将其转换为 CUE 格式的文件。

所以，为了能够使用 Kubernetes 中 `Deployment` 和 `Serivice` 资源，我们需要下载并转换为 `core` 和 `apps` Kubernetes 模块的 CUE 定义，如下所示：

```shell
cue get go k8s.io/api/core/v1
cue get go k8s.io/api/apps/v1
```

随后，该模块目录下可以看到如下结构：

```shell
├── cue.mod
│   ├── gen
│   │   └── k8s.io
│   │       ├── api
│   │       │   ├── apps
│   │       │   └── core
│   │       └── apimachinery
│   │           └── pkg
│   ├── module.cue
│   ├── pkg
│   └── usr
├── def.cue
├── go.mod
└── go.sum
```

该包在 CUE 模版中被导入的路径应该是：

```cue
import (
   apps "k8s.io/api/apps/v1"
   corev1 "k8s.io/api/core/v1"
)
```

3. 重构目录结构

我们的目标是本地测试模版并在 KubeVela 中使用相同模版。
所以我们需要对我们本地 CUE 模块目录进行一些重构，并将目录与 KubeVela 提供的导入路径保持一致。

我们将 `apps` 和 `core` 目录从 `cue.mod/gen/k8s.io/api` 复制到 `cue.mod/gen/k8s.io`。
请注意，我们应将源目录 `apps` 和 `core` 保留在 `gen/k8s.io/api` 中，以避免出现包依赖性问题。

```bash
cp -r cue.mod/gen/k8s.io/api/apps cue.mod/gen/k8s.io
cp -r cue.mod/gen/k8s.io/api/core cue.mod/gen/k8s.io
```

合并过之后到目录结构如下：

```shell
├── cue.mod
│   ├── gen
│   │   └── k8s.io
│   │       ├── api
│   │       │   ├── apps
│   │       │   └── core
│   │       ├── apimachinery
│   │       │   └── pkg
│   │       ├── apps
│   │       └── core
│   ├── module.cue
│   ├── pkg
│   └── usr
├── def.cue
├── go.mod
└── go.sum
```

因此，您可以使用与 KubeVela 对齐的路径导入包：

```cue
import (
   apps "k8s.io/apps/v1"
   corev1 "k8s.io/core/v1"
)
```

4. 运行测试

最终，我们可以使用 `Kube` 包测试 CUE 模版。

```cue
import (
   apps "k8s.io/apps/v1"
   corev1 "k8s.io/core/v1"
)

// output is validated by Deployment.
output: apps.#Deployment
output: {
	metadata: {
		name:      context.name
		namespace: "default"
	}
	spec: {
		selector: matchLabels: {
			"app": context.name
		}
		template: {
			metadata: {
				labels: {
					"app":     context.name
					"version": parameter.version
				}
			}
			spec: {
				terminationGracePeriodSeconds: parameter.podShutdownGraceSeconds
				containers: [{
					name:  context.name
					image: parameter.image
					ports: [{
						if parameter.containerPort != _|_ {
							containerPort: parameter.containerPort
						}
						if parameter.containerPort == _|_ {
							containerPort: parameter.servicePort
						}
					}]
					if parameter.env != _|_ {
						env: [
							for k, v in parameter.env {
								name:  k
								value: v
							},
						]
					}
					resources: {
						requests: {
							if parameter.cpu != _|_ {
								cpu: parameter.cpu
							}
							if parameter.memory != _|_ {
								memory: parameter.memory
							}
						}
					}
				}]
			}
		}
	}
}

outputs:{
  service: corev1.#Service
}


// Service
outputs: service: {
	metadata: {
		name: context.name
		labels: {
			"app": context.name
		}
	}
	spec: {
		//type: "ClusterIP"
		selector: {
			"app": context.name
		}
		ports: [{
			port: parameter.servicePort
			if parameter.containerPort != _|_ {
				targetPort: parameter.containerPort
			}
			if parameter.containerPort == _|_ {
				targetPort: parameter.servicePort
			}
		}]
	}
}
parameter: {
	version:        *"v1" | string
	image:          string
	servicePort:    int
	containerPort?: int
	// +usage=Optional duration in seconds the pod needs to terminate gracefully
	podShutdownGraceSeconds: *30 | int
	env: [string]: string
	cpu?:    string
	memory?: string
}

// mock context data
context: {
    name: "test"
}

// mock parameter data
parameter: {
	image:          "test-image"
	servicePort:    8000
	env: {
        "HELLO": "WORLD"
    }
}
```

使用 `cue export` 导出渲染结果。

```shell
$ cue export def.cue --out yaml
output:
  metadata:
    name: test
    namespace: default
  spec:
    selector:
      matchLabels:
        app: test
    template:
      metadata:
        labels:
          app: test
          version: v1
      spec:
        terminationGracePeriodSeconds: 30
        containers:
        - name: test
          image: test-image
          ports:
          - containerPort: 8000
          env:
          - name: HELLO
            value: WORLD
          resources:
            requests: {}
outputs:
  service:
    metadata:
      name: test
      labels:
        app: test
    spec:
      selector:
        app: test
      ports:
      - port: 8000
        targetPort: 8000
parameter:
  version: v1
  image: test-image
  servicePort: 8000
  podShutdownGraceSeconds: 30
  env:
    HELLO: WORLD
context:
  name: test
```

## Dry-Run `Application`

当 CUE 模版就绪，我们就可以使用 `vela system dry-run` 执行 dry-run 并检查在真实 Kubernetes 集群中被渲染的资源。该命令行背后的执行逻辑与 KubeVela 中 `Application` 控制器的逻辑是一致的。

首先，我们需要使用 `mergedef.sh` 合并 Definition 和 CUE 文件。

```shell
$ mergedef.sh def.yaml def.cue > componentdef.yaml
```

随后，我们创建 `test-app.yaml` Application。

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: boutique
  namespace: default
spec:
  components:
    - name: frontend
      type: microservice
      properties:
        image: registry.cn-hangzhou.aliyuncs.com/vela-samples/frontend:v0.2.2
        servicePort: 80
        containerPort: 8080
        env:
          PORT: "8080"
        cpu: "100m"
        memory: "64Mi"
```

针对上面 Application 使用 `vela system dry-run` 命令执行 dry-run 操作。

```shell
$ vela system dry-run -f test-app.yaml -d componentdef.yaml
---
# Application(boutique) -- Comopnent(frontend)
---

apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app.oam.dev/component: frontend
    app.oam.dev/name: boutique
    workload.oam.dev/type: microservice
  name: frontend
  namespace: default
spec:
  selector:
    matchLabels:
      app: frontend
  template:
    metadata:
      labels:
        app: frontend
        version: v1
    spec:
      containers:
      - env:
        - name: PORT
          value: "8080"
        image: registry.cn-hangzhou.aliyuncs.com/vela-samples/frontend:v0.2.2
        name: frontend
        ports:
        - containerPort: 8080
        resources:
          requests:
            cpu: 100m
            memory: 64Mi
      serviceAccountName: default
      terminationGracePeriodSeconds: 30

---
apiVersion: v1
kind: Service
metadata:
  labels:
    app: frontend
    app.oam.dev/component: frontend
    app.oam.dev/name: boutique
    trait.oam.dev/resource: service
    trait.oam.dev/type: AuxiliaryWorkload
  name: frontend
spec:
  ports:
  - port: 80
    targetPort: 8080
  selector:
    app: frontend
  type: ClusterIP

---
```
