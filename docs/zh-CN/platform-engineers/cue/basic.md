---
title:  CUE 入门
---

本章节将详细介绍关于如何使用 CUE 封装和抽象 Kubernetes 中已有的能力。

> 开始阅读本章节前，请确保已经了解 `Application` 资源。

## 概述

KubeVela 将 CUE 作为抽象最优方案的主要原因如下：

- **CUE 本身就是为大规模配置而设计。** CUE 能够感知非常复杂的配置文件，并且能够安全地更改可修改配置中成千上万个对象的值。这非常符合 KubeVela 的最初目标，即以 web-scale 方式定义和交付生产级别的应用程序（web-scale，是一种软件设计方法，主要包含可扩展性、一致性、容忍度和版本控制等）。
-  **CUE 支持一流的代码生成和自动化。** CUE 原生支持与现有工具以及工作流进行集成，反观其他工具则需要自定义复杂的方案才能实现。例如，需要手动使用 Go 代码生成 OpenAPI 模式。KubeVela 也是依赖 CUE 该特性进行构建开发工具和GUI界面。
- **CUE与Go完美集成。** KubeVela 像 Kubernetes 系统中的大多数项目一样使用 GO 进行开发。CUE 已经在 Go 中实现并提供了丰富的 API 。 KubeVela 以 CUE 为核心实现 Kubernetes 控制器。 借助 CUE KubeVela 可以轻松处理数据约束问题。

> 更多细节请查看 [The Configuration Complexity Curse](https://blog.cedriccharly.com/post/20191109-the-configuration-complexity-curse/) 以及 [The Logic of CUE](https://cuelang.org/docs/concepts/logic/)。

## 前提

请确保你的环境中已经安装如下命令行：
* [`cue` >=v0.2.2](https://cuelang.org/docs/install/)

## CUE 命令行基础

我们可以使用几乎相同的格式在同一个文件中定义模型和数据，以下为 CUE 基础数据类型：

```
a: 1.5
a: float
b: 1
b: int
d: [1, 2, 3]
g: {
	h: "abc"
}
e: string
```

CUE 是 JSON 的超集， 我们可以像使用 json 一样使用 CUE，同时具备以下便利性：

* C 样式的注释，
* 字段名称可以省略引号且不带特殊字符，
* 字段末尾逗号可选，
* 允许列表中最后一个元素末尾带逗号，
* 外花括号可选。

CUE 拥有强大的命令行。请将数据保存到 `first.cue` 文件并尝试使用命令行。

* 格式化 CUE 文件。如果你使用 Goland 或者类似 JetBrains IDE，
  可以参考该文章配置自动格式化插件 [使用 Goland 设置 cuelang 的自动格式化](https://wonderflow.info/posts/2020-11-02-goland-cuelang-format/)。
  该命令不仅可以格式化 CUE 文件，还能指出错误的模型，相当好用的命令。
    ```shell
    cue fmt first.cue
    ```

* 模型校验。 除了 `cue fmt`，你还可以使用 `vue vet` 来校验模型.
    ```shell
    cue vet first.cue
    ```

* 计算/渲染结果。 `cue eval` 可以计算 CUE 文件并且渲染出最终结果。
  我们看到最终结果中并不包含 `a: float` 和 `b: int`，这是因为这两个变量已经被计算填充。
  其中 `e: string` 没有被明确的赋值, 故保持不变.
    ```shell
   $ cue eval first.cue
    a: 1.5
    b: 1
    d: [1, 2, 3]
    g: {
    h: "abc"
    }
    e: string
    ```

* 渲染指定结果。例如，我们仅想知道文件中 `b` 的渲染结果，则可以使用该参数 `-e`。
    ```shell
    $ cue eval -e b first.cue
    1
    ```

* 导出渲染结果。 `cue export` 可以导出最终渲染结果。如果一些变量没有被定义执行该命令将会报错。
    ```shell
    $ cue export first.cue
    e: cannot convert incomplete value "string" to JSON:
        ./first.cue:9:4
    ```
  我们可以通过给 `e` 赋值来完成赋值，例如：
    ```shell
    echo "e: \"abc\"" >> first.cue
    ```
  然后，该命令就可以正常工作。默认情况下, 渲染结果会被格式化为 json 格式。
    ```shell
    $ cue export first.cue
    {
        "a": 1.5,
        "b": 1,
        "d": [
            1,
            2,
            3
        ],
        "g": {
            "h": "abc"
        },
        "e": "abc"
    }
    ```

* 导出 YAML 格式渲染结果。
    ```shell
    $ cue export first.cue --out yaml
    a: 1.5
    b: 1
    d:
    - 1
    - 2
    - 3
    g:
      h: abc
    e: abc
    ```

* 导出指定变量的结果。
    ```shell
    $ cue export -e g first.cue
    {
        "h": "abc"
    }
    ```

至此, 你已经学习完所有常用 CUE 命令行参数。

## CUE 语言基础

* 数据类型： 以下为 CUE 的基础数据类型。

```shell
// float
a: 1.5

// int
b: 1

// string
c: "blahblahblah"

// array
d: [1, 2, 3, 1, 2, 3, 1, 2, 3]

// bool
e: true

// struct
f: {
	a: 1.5
	b: 1
	d: [1, 2, 3, 1, 2, 3, 1, 2, 3]
	g: {
		h: "abc"
	}
}

// null
j: null
```

* 自定义 CUE 类型。你可以使用 `#` 符号来指定一些表示 CUE 类型的变量。

```
#abc: string
```

我们将上述内容保存到 `second.cue` 文件。 执行 `cue export` 不会报 `#abc` 是一个类型不完整的值。

```shell
$ cue export second.cue
{}
```

你还可以定义更复杂的自定义结构，比如：

```
#abc: {
  x: int
  y: string
  z: {
    a: float
    b: bool
  }
}
```

自定义结构在 KubeVela 中被广泛用于定义模板和进行验证。

## CUE 模板和引用

我们开始尝试利用刚刚学习知识来定义 CUE 模版。

1. 定义结构体变量 `parameter`.

```shell
parameter: {
	name: string
	image: string
}
```

保存上述变量到文件 `deployment.cue`.

2. 定义更复杂的结构变量 `template` 同时引用变量 `parameter`.

```
template: {
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

熟悉 Kubernetes 的人可能已经知道，这是 Kubernetes Deployment 的模板。 `parameter` 为模版的参数部分。

添加上述内容到文件 `deployment.cue`.

4. 随后, 我们通过添加以下内容来完成变量赋值:

```
parameter:{
   name: "mytest"
   image: "nginx:v1"
}
```

5. 最后, 导出渲染结果为 yaml 格式:

```shell
$ cue export deployment.cue -e template --out yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: mytest
        image: nginx:v1
    metadata:
      labels:
        app.oam.dev/component: mytest
  selector:
    matchLabels:
      app.oam.dev/component: mytest
```

## 高级 CUE 设计

* 开放的结构体和列表。在列表或者结构体中使用 `...` 说明该对象为开放的。
   -  列表对象 `[...string]` ，说明该对象可以容纳多个字符串元素。
      如果不添加 `...`, 该对象 `[string]` 说明列表只能容纳一个类型为 `string` 的元素。
   -  如下所示的结构体说明可以包含未知字段。
      ```
      {
        abc: string   
        ...
      }
      ```

* 运算符  `|`, 它可以表示两种类型的值。如下所示，变量 `a` 表示类型可以是字符串或者整数类型。

```shell
a: string | int
```

* 默认值， 我们可以使用符号 `*` 定义变量的默认值。通常与符号 `|` 配合使用，
  代表某种类型的默认值。如下所示，变量 `a` 类型为 `int`，默认值为 `1`。

```shell
a: *1 | int
```

* 选填变量。 某些情况下，一些变量不一定被使用，这些变量就是可选变量，我们可以使用 `?:` 定义此类变量。
  如下所示, `a` 是可选变量, 自定义 `#my` 对象中 `x` 和 `z` 为可选变量， 而 `y` 为必填字段。

```
a ?: int

#my: {
x ?: string
y : int
z ?:float
}
```

选填变量可以被跳过，这经常和条件判断逻辑一起使用。
具体来说，如果某些字段不存在，则 CUE 语法为 `if _variable_！= _ | _` ，如下所示：

```
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

* 运算符  `&`，该运算符用来运算两个变量。

```shell
a: *1 | int
b: 3
c: a & b
```

保存上述内容到 `third.cue` 文件。

你可以使用 `cue eval` 来验证结果：

```shell
$ cue eval third.cue
a: 1
b: 3
c: 3
```

* 条件判断。 当你执行一些级联操作时，不同的值会影响不同的结果，条件判断就非常有用。
  因此，你可以在模版中执行 `if..else` 的逻辑。

```shell
price: number
feel: *"good" | string
// Feel bad if price is too high
if price > 100 {
    feel: "bad"
}
price: 200
```

保存上述内容到 `fourth.cue` 文件。

你可以使用 `cue eval` 来验证结果：

```shell
$ cue eval fourth.cue
price: 200
feel:  "bad"
```

另一个示例是将布尔类型作为参数。

```
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


* For循环。 我们为了避免重复可以使用 for 循环。
  - Map 循环
    ```cue
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
  - 类型循环
    ```
    #a: {
        "hello": "Barcelona"
        "nihao": "Shanghai"
    }
    
    for k, v in #a {
        "\(k)": {
            nameLen: len(v)
            value:   v
        }
    }
    ```
  - 切片循环
    ```cue
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

备注， 可以使用 `"\( _my-statement_ )"` 进行字符串内部计算，比如上面类型循环示例中，获取值的长度等等操作。

## 导入 CUE 内部包

CUE 有很多 [internal packages](https://pkg.go.dev/cuelang.org/go@v0.2.2/pkg) 可以被 KubeVela 使用。

如下所示，使用 `strings.Join` 方法将字符串列表拼接成字符串。

```cue
import ("strings")

parameter: {
	outputs: [{ip: "1.1.1.1", hostname: "xxx.com"}, {ip: "2.2.2.2", hostname: "yyy.com"}]
}
output: {
	spec: {
		if len(parameter.outputs) > 0 {
			_x: [ for _, v in parameter.outputs {
				"\(v.ip) \(v.hostname)"
			}]
			message: "Visiting URL: " + strings.Join(_x, "")
		}
	}
}
```

## 导入 Kubernetes 包

KubeVela 会从 Kubernetes 集群中读取 OpenApi，并将 Kubernetes 所有资源自动构建为内部包。

你可以在 KubeVela 的 CUE 模版中通过 `kube/<apiVersion>` 导入这些包，就像使用 CUE 内部包一样。

比如，`Deployment` 可以这样使用：

```cue
import (
   apps "kube/apps/v1"
)

parameter: {
    name:  string
}

output: apps.#Deployment
output: {
    metadata: name: parameter.name
}
```

`Service` 可以这样使用（无需使用别名导入软件包）：

```cue
import ("kube/v1")

output: v1.#Service
output: {
	metadata: {
		"name": parameter.name
	}
	spec: type: "ClusterIP",
}

parameter: {
	name:  "myapp"
}
```

甚至已经安装的 CRD 也可以正常使用：

```
import (
  oam  "kube/core.oam.dev/v1alpha2"
)

output: oam.#Application
output: {
	metadata: {
		"name": parameter.name
	}
}

parameter: {
	name:  "myapp"
}
```