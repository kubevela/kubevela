# 如何在 20 分钟内给你的 K8s PaaS 上线一个新功能

>2020年12月14日 19:33, by @wonderflow

上个月，[KubeVela 正式发布](https://kubevela.io/#/blog/zh/kubevela-the-extensible-app-platform-based-on-open-application-model-and-kubernetes)了，
作为一款简单易用且高度可扩展的应用管理平台与核心引擎，可以说是广大平台工程师用来构建自己的云原生 PaaS 的神兵利器。
那么本文就以一个实际的例子，讲解一下如何在 20 分钟内，为你基于 KubeVela 的 PaaS “上线“一个新能力。

在正式开始本文档的教程之前，请确保你本地已经正确[安装了 KubeVela](https://kubevela.io/#/en/install) 及其依赖的 K8s 环境。

# KubeVela 扩展的基本结构

KubeVela 的基本架构如图所示：

![image](https://kubevela-docs.oss-cn-beijing.aliyuncs.com/kubevela-extend.jpg)

简单来说，KubeVela 通过添加 **Workload Type** 和 **Trait** 来为用户扩展能力，平台的服务提供方通过 Definition 文件注册和扩展，向上通过 Appfile 透出扩展的功能。官方文档中也分别给出了基本的编写流程，其中2个是Workload的扩展例子，一个是Trait的扩展例子：

- [OpenFaaS 为例的 Workload Type 扩展](https://kubevela.io/#/en/platform-engineers/workload-type)
- [云资源 RDS 为例的 Workload Type 扩展](https://kubevela.io/#/en/platform-engineers/cloud-services)
- [KubeWatch 为例的 Trait 扩展](https://kubevela.io/#/en/platform-engineers/trait)

我们以一个内置的 WorkloadDefinition 为例来介绍一下 Definition 文件的基本结构：

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: webservice
  annotations:
    definition.oam.dev/description: "`Webservice` is a workload type to describe long-running, scalable, containerized services that have a stable network endpoint to receive external network traffic from customers.
    If workload type is skipped for any service defined in Appfile, it will be defaulted to `Web Service` type."
spec:
  definitionRef:
    name: deployments.apps
  extension:
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
      					if parameter["env"] != _|_ {
      						env: parameter.env
      					}
      					if context["config"] != _|_ {
      						env: context.config
      					}
      					ports: [{
      						containerPort: parameter.port
      					}]
      					if parameter["cpu"] != _|_ {
      						resources: {
      							limits:
      								cpu: parameter.cpu
      							requests:
      								cpu: parameter.cpu
      						}}
      				}]
      		}}}
      }
      parameter: {
      	// +usage=Which image would you like to use for your service
      	// +short=i
      	image: string
      
      	// +usage=Commands to run in the container
      	cmd?: [...string]
      
      	// +usage=Which port do you want customer traffic sent to
      	// +short=p
      	port: *80 | int
      	// +usage=Define arguments by using environment variables
      	env?: [...{
      		// +usage=Environment variable name
      		name: string
      		// +usage=The value of the environment variable
      		value?: string
      		// +usage=Specifies a source the value of this var should come from
      		valueFrom?: {
      			// +usage=Selects a key of a secret in the pod's namespace
      			secretKeyRef: {
      				// +usage=The name of the secret in the pod's namespace to select from
      				name: string
      				// +usage=The key of the secret to select from. Must be a valid secret key
      				key: string
      			}
      		}
      	}]
      	// +usage=Number of CPU units for the service, like `0.5` (0.5 CPU core), `1` (1 CPU core)
      	cpu?: string
      }
```

乍一看挺长的，好像很复杂，但是不要着急，其实细看之下它分为两部分：

* 不含扩展字段的 Definition 注册部分。
* 供 Appfile 使用的扩展模板（CUE Template）部分 

我们拆开来慢慢介绍，其实学起来很简单。

# 不含扩展字段的 Definition 注册部分

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: webservice
  annotations:
    definition.oam.dev/description: "`Webservice` is a workload type to describe long-running, scalable, containerized services that have a stable network endpoint to receive external network traffic from customers.
    If workload type is skipped for any service defined in Appfile, it will be defaulted to `Web Service` type."
spec:
  definitionRef:
    name: deployments.apps
```

这一部分满打满算11行，其中有3行是在介绍 `webservice`的功能，5行是固定的格式。只有2行是有特定信息：

```yaml
  definitionRef:
    name: deployments.apps
```

这两行的意思代表了这个Definition背后用的CRD名称是什么，其格式是 `<resources>.<api-group>`.了解 K8s 的同学应该知道 K8s 中比较常用的是通过 `api-group`, `version` 和 `kind` 定位资源，而 `kind` 在 K8s restful API 中对应的是 `resources`。以大家熟悉 `Deployment` 和 `ingress` 为例，它的对应关系如下：


| api-group | kind | version | resources | 
| -------- | -------- | -------- | -------- |
| apps     | Deployment     | v1     | deployments |
|  networking.k8s.io | Ingress | v1 | ingresses | 

> 这里补充一个小知识，为什么有了 kind 还要加个 resources 的概念呢？
> 因为一个 CRD 除了 kind 本身还有一些像 status，replica 这样的字段希望跟 spec 本身解耦开来在 restful API 中单独更新，
> 所以 resources 除了 kind 对应的那一个，还会有一些额外的 resources，如 Deployment 的 status 表示为 `deployments/status`。

所以相信聪明的你已经明白了不含 extension 的情况下，Definition应该怎么写了，最简单的就是根据 K8s 的资源组合方式拼接一下，只要填下面三个尖括号的空格就可以了。

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: <这里写名称>
spec:
  definitionRef:
    name: <这里写resources>.<这里写api-group>
```

针对运维特征注册（TraitDefinition）也是这样。

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  name: <这里写名称>
spec:
  definitionRef:
    name: <这里写resources>.<这里写api-group>
```

所以把 `Ingress` 作为 KubeVela 的扩展写进去就是：

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  name:  ingress
spec:
  definitionRef:
    name: ingresses.networking.k8s.io
```

除此之外，TraitDefinition 中还增加了一些其他功能模型层功能，如：

* `appliesToWorkloads`: 表示这个 trait 可以作用于哪些 Workload 类型。
* `conflictWith`： 表示这个 trait 和哪些其他类型的 trait 有冲突。
* `workloadRefPath`： 表示这个 trait 包含的 workload 字段是哪个，KubeVela 在生成 trait 对象时会自动填充。
... 

这些功能都是可选的，本文中不涉及使用，在后续的其他文章中我们再给大家详细介绍。

所以到这里，相信你已经掌握了一个不含 extensions 的基本扩展模式，而剩下部分就是围绕 [CUE](https://cuelang.org/) 的抽象模板。

# 供 Appfile 使用的扩展模板（CUE Template）部分 

对 CUE 本身有兴趣的同学可以参考这篇[CUE 基础入门](https://wonderflow.info/posts/2020-12-15-cuelang-template/) 多做一些了解，限于篇幅本文对 CUE 本身不详细展开。

大家知道 KubeVela 的 Appfile 写起来很简洁，但是 K8s 的对象是一个相对比较复杂的 YAML，而为了保持简洁的同时又不失可扩展性，KubeVela 提供了一个从复杂到简洁的桥梁。
这就是 Definition 中 CUE Template 的作用。

## CUE 格式模板

让我们先来看一个 Deployment 的 YAML 文件，如下所示，其中很多内容都是固定的框架（模板部分），真正需要用户填的内容其实就少量的几个字段（参数部分）。

```yaml
apiVersion: apps/v1
kind: Deployment
meadata:
  name: mytest
spec:
  template:
    spec:
      containers:
      - name: mytest
        env:
        - name: a
          value: b
        image: nginx:v1
    metadata:
      labels:
        app.oam.dev/component: mytest
  selector:
    matchLabels:
      app.oam.dev/component: mytest
```

在 KubeVela 中，Definition 文件的固定格式就是分为 `output` 和 `parameter` 两部分。其中`output`中的内容就是“模板部分”，而 `parameter` 就是参数部分。

那我们来把上面的 Deployment YAML 改写成 Definition 中模板的格式。

```cue
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: name: "mytest"
	spec: {
		selector: matchLabels: {
			"app.oam.dev/component": "mytest"
		}
		template: {
			metadata: labels: {
				"app.oam.dev/component": "mytest"
			}
			spec: {
				containers: [{
					name:  "mytest"
					image: "nginx:v1"
					env: [{name:"a",value:"b"}]
				}]
			}}}
}
```

这个格式跟 json 很像，事实上这个是 CUE 的格式，而 CUE 本身就是一个 json 的超集。也就是说，CUE的格式在满足 JSON 规则的基础上，增加了一些简便规则，
使其更易读易用：

* C 语言的注释风格。
* 表示字段名称的双引号在没有特殊符号的情况下可以缺省。
* 字段值结尾的逗号可以缺省，在字段最后的逗号写了也不会出错。
* 最外层的大括号可以省略。

## CUE 格式的模板参数--变量引用

编写好了模板部分，让我们来构建参数部分，而这个参数其实就是变量的引用。

```
parameter: {
	name: string
	image: string
}
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	spec: {
		selector: matchLabels: {
			"app.oam.dev/component": parameter.name
		}
		template: {
			metadata: labels: {
				"app.oam.dev/component": parameter.name
			}
			spec: {
				containers: [{
					name:  parameter.name
					image: parameter.image
				}]
			}}}
}
```

如上面的这个例子所示，KubeVela 中的模板参数就是通过 `parameter` 这个部分来完成的，而`parameter` 本质上就是作为引用，替换掉了 `output` 中的某些字段。

## 完整的 Definition 以及在 Appfile 使用

事实上，经过上面两部分的组合，我们已经可以写出一个完整的 Definition 文件：

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: mydeploy
spec:
  definitionRef:
    name: deployments.apps
  extension:
    template: |
        parameter: {
            name: string
            image: string
        }
        output: {
            apiVersion: "apps/v1"
            kind:       "Deployment"
            spec: {
                selector: matchLabels: {
                    "app.oam.dev/component": parameter.name
                }
                template: {
                    metadata: labels: {
                        "app.oam.dev/component": parameter.name
                    }
                    spec: {
                        containers: [{
                            name:  parameter.name
                            image: parameter.image
                        }]
                    }}}
        }
```

为了方便调试，一般情况下可以预先分为两个文件，一部分放前面的 yaml 部分，假设命名为 `def.yaml` 如：

```shell script
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: mydeploy
spec:
  definitionRef:
    name: deployments.apps
  extension:
    template: |
```

另一个则放 cue 文件，假设命名为 `def.cue` ：

```shell script
parameter: {
    name: string
    image: string
}
output: {
    apiVersion: "apps/v1"
    kind:       "Deployment"
    spec: {
        selector: matchLabels: {
            "app.oam.dev/component": parameter.name
        }
        template: {
            metadata: labels: {
                "app.oam.dev/component": parameter.name
            }
            spec: {
                containers: [{
                    name:  parameter.name
                    image: parameter.image
                }]
            }}}
}
```

先对 `def.cue` 做一个格式化，格式化的同时 cue 工具本身会做一些校验，也可以更深入的[通过 cue 命令做调试](https://wonderflow.info/posts/2020-12-15-cuelang-template/):

```shell script
cue fmt def.cue
```

调试完成后，可以通过脚本把这个 yaml 组装：

```shell script
./hack/vela-templates/mergedef.sh def.yaml def.cue > mydeploy.yaml
```

再把这个 yaml 文件 apply 到 K8s 集群中。

```shell script
$ kubectl apply -f mydeploy.yaml
workloaddefinition.core.oam.dev/mydeploy created
```

一旦新能力 `kubectl apply` 到了 Kubernetes 中，不用重启，也不用更新，KubeVela 的用户可以立刻看到一个新的能力出现并且可以使用了：

```shell script
$ vela worklaods
Automatically discover capabilities successfully ✅ Add(1) Update(0) Delete(0)

TYPE       	CATEGORY	DESCRIPTION
+mydeploy  	workload	description not defined

NAME    	DESCRIPTION
mydeploy	description not defined
```

在 Appfile 中使用方式如下：

```yaml
name: my-extend-app
services:
  mysvc:
    type: mydeploy
    image: crccheck/hello-world
    name: mysvc
```

执行 `vela up` 就能把这个运行起来了：

```shell script
$ vela up -f docs/examples/blog-extension/my-extend-app.yaml
Parsing vela appfile ...
Loading templates ...

Rendering configs for service (mysvc)...
Writing deploy config to (.vela/deploy.yaml)

Applying deploy configs ...
Checking if app has been deployed...
App has not been deployed, creating a new deployment...
✅ App has been deployed 🚀🚀🚀
    Port forward: vela port-forward my-extend-app
             SSH: vela exec my-extend-app
         Logging: vela logs my-extend-app
      App status: vela status my-extend-app
  Service status: vela status my-extend-app --svc mysvc
```

我们来查看一下应用的状态，已经正常运行起来了（`HEALTHY Ready: 1/1`）：

```shell script
$ vela status my-extend-app
About:

  Name:      	my-extend-app
  Namespace: 	env-application
  Created at:	2020-12-15 16:32:25.08233 +0800 CST
  Updated at:	2020-12-15 16:32:25.08233 +0800 CST

Services:

  - Name: mysvc
    Type: mydeploy
    HEALTHY Ready: 1/1
```

# Definition 模板中的高级用法

上面我们已经通过模板替换这个最基本的功能体验了扩展 KubeVela 的全过程，除此之外，可能你还有一些比较复杂的需求，如条件判断，循环，复杂类型等，需要一些高级的用法。

## 结构体参数

如果模板中有一些参数类型比较复杂，包含结构体和嵌套的多个结构体，就可以使用结构体定义。

1. 定义一个结构体类型，包含1个字符串成员、1个整型和1个结构体成员。
```
#Config: {
    name:  string
    value: int
    other: {
      key: string
      value: string
    }
}
```

2. 在变量中使用这个结构体类型，并作为数组使用。
```
parameter: {
	name: string
	image: string
	config: [...#Config]
}
```

3. 同样的目标中也是以变量引用的方式使用。
```shell script
output: {
      ...
			spec: {
				containers: [{
					name:  parameter.name
					image: parameter.image
					env: parameter.config
				}]
			}
       ...
}
```

4. Appfile 中的写法就是按照 parameter 定义的结构编写。
```
name: my-extend-app
services:
  mysvc:
    type: mydeploy
    image: crccheck/hello-world
    name: mysvc
    config:
    - name: a
      value: 1
      other:
        key: mykey
        value: myvalue
```

## 条件判断

有时候某些参数加还是不加取决于某个条件：

```shell script
parameter: {
	name:   string
	image:  string
	useENV: bool
}
output: {
    ...
	spec: {
		containers: [{
			name:  parameter.name
			image: parameter.image
			if parameter.useENV == true {
				env: [{name: "my-env", value: "my-value"}]
			}
		}]
	}
    ...
}
```

在 Appfile 就是写值。
```
name: my-extend-app
services:
  mysvc:
    type: mydeploy
    image: crccheck/hello-world
    name: mysvc
    useENV: true
```

## 可缺省参数

有些情况下参数可能存在也可能不存在，即非必填，这个时候一般要配合条件判断使用，对于某个字段不存在的情况，判断条件是是 `_variable != _|_`。

```shell script
parameter: {
	name: string
	image: string
	config?: [...#Config]
}
output: {
    ...
	spec: {
		containers: [{
			name:  parameter.name
			image: parameter.image
			if parameter.config != _|_ {
				config: parameter.config
			}
		}]
	}
    ...
}
```

这种情况下 Appfile 的 config 就非必填了，填了就渲染，没填就不渲染。

## 默认值

对于某些参数如果希望设置一个默认值，可以采用这个写法。

```shell script
parameter: {
	name: string
	image: *"nginx:v1" | string
}
output: {
    ...
	spec: {
		containers: [{
			name:  parameter.name
			image: parameter.image
		}]
	}
    ...
}
```

这个时候 Appfile 就可以不写 image 这个参数，默认使用 "nginx:v1"：

```
name: my-extend-app
services:
  mysvc:
    type: mydeploy
    name: mysvc
```


## 循环

### Map 类型的循环

```shell script
parameter: {
	name:  string
	image: string
	env: [string]: string
}
output: {
	spec: {
		containers: [{
			name:  parameter.name
			image: parameter.image
			env: [
				for k, v in parameter.env {
					name:  k
					value: v
				},
			]
		}]
	}
}
```

Appfile 中的写法：
```
name: my-extend-app
services:
  mysvc:
    type: mydeploy
    name:  "mysvc"
    image: "nginx"
    env:
      env1: value1
      env2: value2
```
     
### 数组类型的循环

```shell script
parameter: {
	name:  string
	image: string
	env: [...{name:string,value:string}]
}
output: {
  ...
 	spec: {
		containers: [{
			name:  parameter.name
			image: parameter.image
			env: [
				for _, v in parameter.env {
					name:  v.name
					value: v.value
				},
			]
		}]
	}
}
```

Appfile 中的写法：
```
name: my-extend-app
services:
  mysvc:
    type: mydeploy
    name:  "mysvc"
    image: "nginx"
    env:
    - name: env1
      value: value1
    - name: env2
      value: value2
```

## KubeVela 内置的 `context` 变量

大家可能也注意到了，我们在 parameter 中定义的 name 每次在 Appfile中 实际上写了两次，一次是在 services 下面（每个service都以名称区分），
另一次则是在具体的`name`参数里面。事实上这里重复的不应该由用户再写一遍，所以 KubeVela 中还定义了一个内置的 `context`，里面存放了一些通用的环境上下文信息，如应用名称、秘钥等。
直接在模板中使用 context 就不需要额外增加一个 `name` 参数了， KubeVela 在运行渲染模板的过程中会自动传入。

```shell script
parameter: {
	image: string
}
output: {
  ...
	spec: {
		containers: [{
			name:  context.name
			image: parameter.image
		}]
	}
  ...
}
```

## KubeVela 中的注释增强

KubeVela 还对 cuelang 的注释做了一些扩展，方便自动生成文档以及被 CLI 使用。

```
 parameter: {
      	// +usage=Which image would you like to use for your service
      	// +short=i
      	image: string
      
      	// +usage=Commands to run in the container
      	cmd?: [...string]
       ...
      }
```

其中，`+usgae` 开头的注释会变成参数的说明，`+short` 开头的注释后面则是在 CLI 中使用的缩写。

# 总结
本文通过实际的案例和详细的讲述，为你介绍了在 KubeVela 中新增一个能力的详细过程与原理，以及能力模板的编写方法。

这里你可能还有个疑问，平台管理员这样添加了一个新能力后，平台的用户又该怎么能知道这个能力怎么使用呢？其实，在 KubeVela 中，它不仅能方便的添加新能力，**它还能自动为“能力”生成 Markdown 格式的使用文档！** 不信，你可以看下 KubeVela 本身的官方网站，所有在 `References/Capabilities`目录下能力使用说明文档（比如[这个](https://kubevela.io/#/en/developers/references/workload-types/webservice)），全都是根据每个能力的模板自动生成的哦。
最后，欢迎大家写一些有趣的扩展功能，提交到 KubeVela 的[社区仓库](https://github.com/oam-dev/catalog/tree/master/registry)中来。 
