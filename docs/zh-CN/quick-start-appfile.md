---
title:  概述
---

为了你的平台获得最佳用户体验，我们建议各位平台构建者们为最终用户提供简单并且友好的 UI，而不是仅仅简单展示全部平台层面的信息。一些常用的做法包括构建 GUI 控制台，使用 DSL，或者创建用户友好的命令行工具。

为了证明在 KubeVela 中提供了良好的构建开发体验，我们开发了一个叫 `Appfile` 的客户端工具。这个工具使得开发者通过一个文件和一个简单的命令：`vela up` 就可以部署任何应用。 

现在，让我们来体验一下它是如何使用的。

## Step 1: 安装

确保你已经参照 [安装指南](./install) 完成了所有的安装验证工作。

## Step 2: 部署你的第一个应用

```bash
$ vela up -f https://raw.githubusercontent.com/oam-dev/kubevela/master/docs/examples/vela.yaml
Parsing vela.yaml ...
Loading templates ...

Rendering configs for service (testsvc)...
Writing deploy config to (.vela/deploy.yaml)

Applying deploy configs ...
Checking if app has been deployed...
App has not been deployed, creating a new deployment...
✅ App has been deployed 🚀🚀🚀
    Port forward: vela port-forward first-vela-app
             SSH: vela exec first-vela-app
         Logging: vela logs first-vela-app
      App status: vela status first-vela-app
  Service status: vela status first-vela-app --svc testsvc
```

检查状态直到看到 `Routes` 为就绪状态:
```bash
$ vela status first-vela-app
About:

  Name:       first-vela-app
  Namespace:  default
  Created at: ...
  Updated at: ...

Services:

  - Name: testsvc
    Type: webservice
    HEALTHY Ready: 1/1
    Last Deployment:
      Created at: ...
      Updated at: ...
    Traits:
      - ✅ ingress: Visiting URL: testsvc.example.com, IP: <your IP address>
```

**在 [kind cluster 配置章节](./install#kind)**, 你可以通过 localhost 访问 service。 在其他配置中, 使用相应的 ingress 地址来替换 localhost。

```
$ curl -H "Host:testsvc.example.com" http://localhost/
<xmp>
Hello World


                                       ##         .
                                 ## ## ##        ==
                              ## ## ## ## ##    ===
                           /""""""""""""""""\___/ ===
                      ~~~ {~~ ~~~~ ~~~ ~~~~ ~~ ~ /  ===- ~~~
                           \______ o          _,/
                            \      \       _,'
                             `'--.._\..--''
</xmp>
```
**瞧!** 你已经基本掌握了它。

## 下一步

- 详细学习 [`Appfile`](./developers/learn-appfile) 并且了解它是如何工作的。
