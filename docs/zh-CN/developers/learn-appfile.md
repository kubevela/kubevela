---
title:  学习使用 Appfile
---

`appfile` 的示例如下:

```yaml
name: testapp

services:
  frontend: # 1st service

    image: oamdev/testapp:v1
    build:
      docker:
        file: Dockerfile
        context: .

    cmd: ["node", "server.js"]
    port: 8080

    route: # trait
      domain: example.com
      rules:
        - path: /testapp
          rewriteTarget: /

  backend: # 2nd service
    type: task # workload type
    image: perl 
    cmd: ["perl",  "-Mbignum=bpi", "-wle", "print bpi(2000)"]
```

在底层，`Appfile` 会从源码构建镜像，然后用镜像名称创建 `application` 资源

## Schema

> 在深入学习 Appfile 的详细 schema 之前，我们建议你先熟悉 KubeVela 的[核心概念](../concepts)

```yaml
name: _app-name_

services:
  _service-name_:
    # If `build` section exists, this field will be used as the name to build image. Otherwise, KubeVela will try to pull the image with given name directly.
    image: oamdev/testapp:v1

    build:
      docker:
        file: _Dockerfile_path_ # relative path is supported, e.g. "./Dockerfile"
        context: _build_context_path_ # relative path is supported, e.g. "."

      push:
        local: kind # optionally push to local KinD cluster instead of remote registry

    type: webservice (default) | worker | task

    # detailed configurations of workload
    ... properties of the specified workload  ...

    _trait_1_:
      # properties of trait 1

    _trait_2_:
      # properties of trait 2

    ... more traits and their properties ...
  
  _another_service_name_: # more services can be defined
    ...
  
```

> 想了解怎样设置特定类型的 workload 或者 trait，请阅读[参考文档手册](./check-ref-doc)

## 示例流程

在以下的流程中，我们会构建并部署一个 NodeJs 的示例 app。该 app 的源文件在[这里](https://github.com/oam-dev/kubevela/tree/master/docs/examples/testapp)。

### 环境要求

- [Docker](https://docs.docker.com/get-docker/) 需要在主机上安装 docker
- [KubeVela](../install) 需要安装 KubeVela 并配置

### 1. 下载测试的 app 的源码

git clone 然后进入 testapp 目录:

```bash
$ git clone https://github.com/oam-dev/kubevela.git
$ cd kubevela/docs/examples/testapp
```

这个示例包含 NodeJs app 的源码和用于构建 app 镜像的Dockerfile

### 2. 使用命令部署 app

我们将会使用目录中的 [vela.yaml](https://github.com/oam-dev/kubevela/tree/master/docs/examples/testapp/vela.yaml) 文件来构建和部署 app

> 注意：请修改 `oamdev` 为你自己注册的账号。或者你可以尝试 `本地测试方式`。

```yaml
    image: oamdev/testapp:v1 # change this to your image
```

执行如下命令：

```bash
$ vela up
Parsing vela.yaml ...
Loading templates ...

Building service (express-server)...
Sending build context to Docker daemon  71.68kB
Step 1/10 : FROM mhart/alpine-node:12
 ---> 9d88359808c3
...

pushing image (oamdev/testapp:v1)...
...

Rendering configs for service (express-server)...
Writing deploy config to (.vela/deploy.yaml)

Applying deploy configs ...
Checking if app has been deployed...
App has not been deployed, creating a new deployment...
✅ App has been deployed 🚀🚀🚀
    Port forward: vela port-forward testapp
             SSH: vela exec testapp
         Logging: vela logs testapp
      App status: vela status testapp
  Service status: vela status testapp --svc express-server
```


检查服务状态：

```bash
$ vela status testapp
  About:
  
    Name:       testapp
    Namespace:  default
    Created at: 2020-11-02 11:08:32.138484 +0800 CST
    Updated at: 2020-11-02 11:08:32.138485 +0800 CST
  
  Services:
  
    - Name: express-server
      Type: webservice
      HEALTHY Ready: 1/1
      Last Deployment:
        Created at: 2020-11-02 11:08:33 +0800 CST
        Updated at: 2020-11-02T11:08:32+08:00
      Routes:

```

#### 本地测试方式

如果你本地有运行的 [kind](../install) 集群，你可以尝试推送到本地。这种方法无需注册远程容器仓库。

在 `build` 中添加 local 的选项值：

```yaml
    build:
      # push image into local kind cluster without remote transfer
      push:
        local: kind

      docker:
        file: Dockerfile
        context: .
```

然后部署到 kind：

```bash
$ vela up
```

<details><summary>(进阶) 检查渲染后的 manifests 文件</summary>

默认情况下，Vela 通过 `./vela/deploy.yaml` 渲染最后的 manifests 文件：

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  name: testapp
  namespace: default
spec:
  components:
  - componentName: express-server
---
apiVersion: core.oam.dev/v1alpha2
kind: Component
metadata:
  name: express-server
  namespace: default
spec:
  workload:
    apiVersion: apps/v1
    kind: Deployment
    metadata:
      name: express-server
    ...
---
apiVersion: core.oam.dev/v1alpha2
kind: HealthScope
metadata:
  name: testapp-default-health
  namespace: default
spec:
  ...
```
</details>

### [可选] 配置其他类型的 workload

至此，我们成功地部署一个默认类型的 workload 的 *[web 服务](../end-user/components/webservice)*。我们也可以添加 *[Task](../end-user/components/task)* 类型的服务到同一个 app 中。

```yaml
services:
  pi:
    type: task
    image: perl 
    cmd: ["perl",  "-Mbignum=bpi", "-wle", "print bpi(2000)"]

  express-server:
    ...
```

然后再次部署 Applfile 来升级应用：

```bash
$ vela up
```

恭喜！你已经学会了使用 `Appfile` 来部署应用了。

## 下一步?

更多关于 app 的操作：
- [Check Application Logs](./check-logs)
- [Execute Commands in Application Container](./exec-cmd)
- [Access Application via Route](./port-forward)

