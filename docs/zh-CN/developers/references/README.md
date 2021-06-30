---
title:  功能参考文档
---

在这篇文档中，我们将展示如何查看给定能力的详细文档 (比如 component 或者 trait)。

这听起来很有挑战，因为每种能力都是 KubeVela 的一个插件(内置能力也是如此)。同时，根据设计， KubeVela 允许平台管理员随时修改功能模板。在这种情况下，我们是否需要为每个新安装的功能手动写文档？ 以及我们如何确保系统的那些文档是最新的？

## 使用浏览器

实际上，作为其可扩展设计的重要组成部分， KubeVela 总是会根据模板的定义对每种 workload 类型或者 Kubernetes 集群注册的 trait 自动生成参考文档。此功能适用于任何功能：内置功能或者你自己的 workload 类型/ traits 。
因此，作为一个终端用户，你唯一需要做的事情是：

```console
$ vela show COMPONENT_TYPE or TRAIT --web
```

这条命令会自动在你的默认浏览器中打开对应的 component 类型或者 traint 参考文档。

以 `$ vela show webservice --web` 为例。 `Web Service` component 类型的详细的文档将立即显示如下：

![](../../resources/vela_show_webservice.jpg)

注意， 在名为 `Specification` 的部分中，它甚至为你提供了一种使用假名称 `my-service-name` 的这种 workload 类型。

同样的， 我们可以执行 `$ vela show autoscale`：

![](../../resources/vela_show_autoscale.jpg)

使用这些自动生成的参考文档，我们可以通过简单的复制粘贴轻松地完成应用程序描述，例如：

```yaml
name: helloworld

services:
  backend: # 复制粘贴上面的 webservice 参考文档
    image: oamdev/testapp:v1
    cmd: ["node", "server.js"]
    port: 8080
    cpu: "0.1"

    autoscale: # 复制粘贴并修改上面的 autoscaler 参考文档
      min: 1
      max: 8
      cron:
        startAt:  "19:00"
        duration: "2h"
        days:     "Friday"
        replicas: 4
        timezone: "America/Los_Angeles"
```

## 使用命令行终端

此参考文档功能也适用于仅有命令行终端的情况，例如：

```shell
$ vela show webservice
# Properties
+-------+----------------------------------------------------------------------------------+---------------+----------+---------+
| NAME  |                                   DESCRIPTION                                    |     TYPE      | REQUIRED | DEFAULT |
+-------+----------------------------------------------------------------------------------+---------------+----------+---------+
| cmd   | Commands to run in the container                                                 | []string      | false    |         |
| env   | Define arguments by using environment variables                                  | [[]env](#env) | false    |         |
| image | Which image would you like to use for your service                               | string        | true     |         |
| port  | Which port do you want customer traffic sent to                                  | int           | true     |      80 |
| cpu   | Number of CPU units for the service, like `0.5` (0.5 CPU core), `1` (1 CPU core) | string        | false    |         |
+-------+----------------------------------------------------------------------------------+---------------+----------+---------+


## env
+-----------+-----------------------------------------------------------+-------------------------+----------+---------+
|   NAME    |                        DESCRIPTION                        |          TYPE           | REQUIRED | DEFAULT |
+-----------+-----------------------------------------------------------+-------------------------+----------+---------+
| name      | Environment variable name                                 | string                  | true     |         |
| value     | The value of the environment variable                     | string                  | false    |         |
| valueFrom | Specifies a source the value of this var should come from | [valueFrom](#valueFrom) | false    |         |
+-----------+-----------------------------------------------------------+-------------------------+----------+---------+


### valueFrom
+--------------+--------------------------------------------------+-------------------------------+----------+---------+
|     NAME     |                   DESCRIPTION                    |             TYPE              | REQUIRED | DEFAULT |
+--------------+--------------------------------------------------+-------------------------------+----------+---------+
| secretKeyRef | Selects a key of a secret in the pod's namespace | [secretKeyRef](#secretKeyRef) | true     |         |
+--------------+--------------------------------------------------+-------------------------------+----------+---------+


#### secretKeyRef
+------+------------------------------------------------------------------+--------+----------+---------+
| NAME |                           DESCRIPTION                            |  TYPE  | REQUIRED | DEFAULT |
+------+------------------------------------------------------------------+--------+----------+---------+
| name | The name of the secret in the pod's namespace to select from     | string | true     |         |
| key  | The key of the secret to select from. Must be a valid secret key | string | true     |         |
+------+------------------------------------------------------------------+--------+----------+---------+
```

## 内置功能

注意，对于所有的内置功能，我们已经将它们的参考文档发布在下面，这些文档遵循同样的文档生成机制。


- Workload Types
	- [webservice](component-types/webservice)
	- [task](component-types/task)
	- [worker](component-types/worker)
- Traits
	- [route](traits/route)
	- [autoscale](traits/autoscale)
	- [rollout](traits/rollout)
	- [metrics](traits/metrics)
	- [scaler](traits/scaler)
