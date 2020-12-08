# KubeVela 正式开源：一个高可扩展的云原生应用平台与核心引擎

>2020年12月7日 12:33, by Lei Zhang and Fei Guo

![image](https://tva1.sinaimg.cn/large/ad5fbf65gy1glgj5q8inej208g049aa6.jpg)

美国西部时间 2020 年 11 月 18 日，在云原生技术“最高盛宴”的 KubeCon 北美峰会 2020 上，CNCF 应用交付领域小组（CNCF SIG App Delivery) 与 Open Application Model (OAM) 社区，以及来自阿里云、微软云的 OAM 项目维护者们在演讲中共同宣布了 KubeVela 开源项目的正式发布。

从 11 月 18 号到 20 号，在为期三天的 KubeCon 北美峰会上有连续 3 场技术演讲，会从不同维度介绍关于 KubeVela 项目的具体细节，其中还包括一个长达 1 个半小时的 KubeVela 互动教学环节。多个重量级组织以如此规模和密度在 KubeCon 北美峰会演讲中介绍一个首次发布的社区开源项目，在 KubeCon 诞生以来并不多见。

KubeVela Github 地址：[https://github.com/oam-dev/kubevela/](https://github.com/oam-dev/kubevela/)

## 什么是 KubeVela ？

一言以蔽之，**KubeVela 是一个简单易用且高度可扩展的应用管理平台与核心引擎**。KubeVela 是基于 Kubernetes 与 OAM 技术构建的。

详细的说，对于应用开发人员来讲，KubeVela 是一个非常低心智负担的云原生应用管理平台，核心功能是让开发人员方便快捷地在 Kubernetes 上定义与交付现代微服务应用，无需了解任何 Kubernetes 本身相关的细节。在这一点上，KubeVela 可以被认为是**云原生社区的 Heroku**。

另一方面，对于平台团队来讲，KubeVela 是一个强大并且高可扩展的云原生应用平台核心引擎。基于这样一个引擎，平台团队可以快速、高效地以 Kubernetes 原生的方式在 KubeVela 中植入任何来自云原生社区的应用管理能力，从而基于 KubeVela 打造出自己需要的云原生平台，比如：云原生数据库 PaaS、云原生 AI 平台、甚至 Serverless 服务。在这一点上，KubeVela 可以被认为是**一个“以应用为中心”的 Kubernetes 发行版**，以 OAM 为核心，让平台团队可以基于 KubeVela 快速打造出属于自己的 PaaS、Serverless 乃至任何面向用户的云原生平台项目。

## KubeVela 解决了什么问题？

现如今，云原生技术的迅猛发展可能让很多人都感觉到眼花缭乱，但实际上，这个生态的总体发展趋势和主旋律，是通过 Kubernetes 搭建了一个统一的基础设施抽象层，为平台团队屏蔽掉了“计算”、“网络”、“存储”等过去我们不得不关注的基础设施概念，使得我们能够基于 Kubernetes 方便地构建出任何我们想要的垂直业务系统而无需关心任何基础设施层的细节。这正是 Kubernetes 被称为云计算界的 Linux 以及 “Platform for Platforms” 的根本原因。

但是，当我们把视角从平台团队提升到垂直业务系统的最终用户（如：应用开发人员）的时候，我们会发现 Kubernetes 这样的定位和设计在解决了平台团队的大问题之后，却也为应用开发者们带来了挑战和困扰。比如，太多的用户都在抱怨 Kubernetes “太复杂了”。究其原因，其实在于 Kubernetes 中的核心概念与体系，如：Pod、sidecar、Service、资源管理、调度算法和 CRD 等等，主要是面向平台团队而非最终用户设计的。缺乏面向用户的设计不仅带来了陡峭的学习曲线，影响了用户的使用体验，拖慢了研发效能，甚至在很多情况下还会引发错误操作乃至生产故障（毕竟不可能每个业务开发人员都是 Kubernetes 专家）。

这也是为什么在云原生生态中，几乎每一个平台团队都会基于 Kubernetes 构建一个上层平台给用户使用。最简单的也会给 Kubernetes 做一个图形界面，稍微正式一些的则往往会基于 Kubernetes 开发一个类 PaaS 平台来满足自己的需求。理论上讲，在 Kubernetes 生态中各种能力已经非常丰富的今天，开发一个类 PaaS 平台应该是比较容易的。

然而现实却往往不尽如人意。在大量的社区访谈当中，我们发现在云原生技术极大普及的今天，基于 Kubernetes 构建一个功能完善、用户友好的上层应用平台，依然是中大型公司们的“专利”。这里的原因在于：

> Kubernetes 生态本身的能力池固然丰富，但是社区里却并没有一个可扩展的、方便快捷的方式，能够帮助平台团队把这些能力快速“组装”成面向最终用户的功能（Feature）。

这种困境带来的结果，就是尽管大家今天都在基于 Kubernetes 在构建上层应用平台，但这些平台本质上却并不能够与 Kubernetes 生态完全打通，而都变成一个又一个的垂直“烟囱”。

在开源社区中，这个问题会更加明显。在今天的 Kubernetes 社区中，不乏各种“面向用户”、“面向应用”的 Kubernetes 上层系统。但正如前文所述，这些平台都无一例外的引入了自己的专属上层抽象、用户界面和插件机制。这里最典型的例子包括经典 PaaS 项目比如 Cloud Foundry，也包括各种 Serverless 平台。作为一个公司的平台团队，我们实际上只有两个选择：要么把自己局限在某种垂直的场景中来适配和采纳某个开源上层平台项目；要么就只能自研一个符合自己诉求的上层平台并且造无数个社区中已经存在的“轮子”。

那么，有没有”第三种选择”能够让平台团队在不造轮子、完全打通 Kubernetes 生态的前提下，轻松的构建面向用户的上层平台呢？

## KubeVela 如何解决上述问题？

KubeVela 项目的创立初衷，就是以一个统一的方式同时解决上述最终用户与平台团队所面临的困境。这也是为何在设计中，KubeVela 对最终用户和平台团队这两种群体进行了单独的画像，以满足他们不同的诉求。

由于 KubeVela 默认的功能集与“Heroku”类似（即：主要面向应用开发人员），所以在下文中，我们会以应用开发人员或者开发者来代替最终用户。但我们很快也会讲到，KubeVela 里的每一个功能，都是一个插件，作为平台团队，你可以轻松地“卸载”它的所有内置能力、然后“安装”自己需要的任何社区能力，让 KubeVela 变成一个完全不一样的系统。

### 1. 应用开发者眼中的 KubeVela

前面已经提到，对于开发者来说，KubeVela 是一个简单、易用、又高可扩展的云原生应用管理工具，它可以让开发者以极低的心智负担和上手成本在 Kubernetes 上定义与部署应用。而关于整个系统的使用，开发者只需要编写一个 docker-compose 风格应用描述文件 Appfile 即可，不需要接触和学习任何 Kubernetes 层的相关细节。

#### 1）一个 Appfile 示例

在下述例子中，我们会将一个叫做 vela.yaml 的 Appfile 放在你的应用代码目录中（比如应用的 GitHub Repo）。这个 Appfile 定义了如何将这个应用编译成 Docker 镜像，如何将镜像部署到 Kubernetes，如何配置外界访问应用的路由和域名，又如何让 Kubernetes 自动根据 CPU 使用量来水平扩展这个应用。

```yaml
name: testapp

services:
    express-server:
      image: oamdev/testapp:v1
      build:
        docker:
          file: Dockerfile
          contrxt: .
      cmd: ["node", "server.js"]
      port: 8080
      cpu: "0.01"

      route:
        domain: example.com
        rules:
          - path: /testapp
            rewriteTarget: /

      autoscale:
        min: 1
        max: 4
        cpuPercent: 5
```

只要有了这个 20 行的配置文件，你接下来唯一需要的事情就是 $ vela up，这个应用就会被部署到 Kubernetes 中然后被外界以 https://example.com/testapp 的方式访问到。

#### 2）Appfile 是如何工作的？

在 KubeVela 的 Appfile 背后，有着非常精妙的设计。首先需要指出的就是，**这个 Appfile 是没有固定的 Schema 的**。

什么意思呢？这个 Appfile 里面你能够填写的每一个字段，都是直接取决于当前平台中有哪些工作负载类型（Workload Types）和应用特征（Traits）是可用的。而熟悉 OAM 的同学都知道，这两个概念，正是 OAM 规范的核心内容，其中：

- [工作负载类型（Workload Type）](https://kubevela.io/#/en/concepts?id=workload-type-amp-trait)，定义的是底层基础设施如何运行这个应用。在上面的例子中，我们声明：名叫 testapp 的应用会启动一个类型为“在线 Web 服务（Web Service）” 的工作负载，其实例的名字是 express-server。

- [应用特征（Traits）](https://kubevela.io/#/en/concepts?id=workload-type-amp-trait)，则为工作负载实例加上了运维时配置。在上面的例子中，我们定义了一个 Route Trait 来描述应用如何被从外界访问，以及一个 Autoscale Trait 来描述应用如何根据 CPU 使用量进行自动的水平扩容。

而正是基于这种模块化的设计，这个 Appfile 本身是高度可扩展的。当任何一个新的 Workload Type 或者 Trait 被安装到平台后，用户就可以立刻在 Appfile 里声明使用这个新增的能力。举个例子，比如后面平台团队新开发了一个用来配置应用监控属性的运维侧能力，叫做 Metrics。那么只需要举手之捞，应用开发者就可以立刻使用 $ vela show metrics 命令查看这个新增能力的详情，并且在 Appfile 中使用它，如下所示：

```yaml
name: testapp

services:
    express-server:
      type: webservice
      image: oamdev/testapp:v1
      build:
        docker:
          file: Dockerfile
          contrxt: .
      cmd: ["node", "server.js"]
      port: 8080
      cpu: "0.01"

      route:
        domain: example.com
        rules:
          - path: /testapp
            rewriteTarget: /

      autoscale:
        min: 1
        max: 4
        cpuPercent: 5

      metrices:
        port: 8080
        path: "/metrics"
        scheme: "http"
        enabled: true
```

这种简单友好、又高度敏捷的使用体验，正是 KubeVela 在最终用户侧提供的主要体感。

#### 3）Vela Up 命令

前面提到，一旦 Appfile 准备好，开发者只需要一句 vela up 命令就可以把整个应用连同它的运维特征部署到 Kubernetes 中。部署成功后，你可以使用 vela status 来查看整个应用的详情，包括如何访问这个应用。

![](https://tvax2.sinaimg.cn/large/ad5fbf65gy1glf9pyhr42j20la0kiafn.jpg)

通过 KubeVela 部署的应用会被自动设置好访问 URL（以及不同版本对应的不同 URL），并且会由 cert-manager 生成好证书。与此同时，KubeVela 还提供了一系列辅助命令（比如：vela logs 和 vela exec）来帮助你在无需成为 Kubernetes 专家的情况下更好地管理和调试你的应用。如果你对上述由 KubeVela 带来的开发者体验感兴趣的话，欢迎前往 KubeVela 项目的用户使用文档来了解更多。

而接下来，我们要切换一下视角，感受一下平台团队眼中的 KubeVela 又是什么样子的。

### 2. 平台工程师眼中的 KubeVela

实际上，前面介绍到的所有开发者侧体验，都离不开 KubeVela 在平台侧进行的各种创新性设计与实现。也正是这些设计的存在，才使得 KubeVela 不是一个简单的 PaaS 或者 Serverless，**而是一个可以由平台工程师扩展成任意垂直系统的云原生平台内核**。

具体来说，KubeVela 为平台工程师提供了三大核心能力，使得基于 Kubernetes 构建上述面向用户的云原生平台从“阳春白雪”变成了“小菜一碟”：

**第一：以应用为中心**。在 Appfile 背后，其实就是“应用”这个概念，它是基于 OAM 模型实现的。通过这样的方式，KubeVela 让“应用”这个概念成为了整个平台对用户暴露的核心 API。KubeVela 中的所有能力，都是围绕着“应用”展开的。这正是为何基于 KubeVela 扩展和构建出来的平台，天然是用户友好的：对于一个开发者来说，他只关心“应用”，而不是容器或者 Kubernetes；而 KubeVela 会确保构建整个平台的过程，也只与应用层的需求有关。

**第二：Kubernetes 原生的高可扩展性**。在前面我们已经提到过，Appfile 是一个由 Workload Type 和 Trait 组成的、完全模块化的对象。而 OAM 模型的一个特点，就是任意一个 Kubernetes API 资源，都可以直接基于 Kubernetes 的 CRD 发现机制注册为一个 Workload Type 或者 Trait。这种可扩展性，使得 KubeVela 并不需要设计任何“插件系统”：**KubeVela 里的每一个能力，都是插件，而整个 Kubernetes 社区，就是 KubeVela 原生的插件中心**。

**第三：简单友好但高度可扩展的用户侧抽象体系**。在了解了 Appfile 之后，你可能已经对这个对象的实现方式产生了好奇。实际上，KubeVela 中并不是简单的实现了一个 Appfile。在平台层，KubeVela 在 OAM 模型层实现中集成了CUELang 这种简洁强大的模板语言，从而为平台工程师基于 Kubernetes API 对象定义用户侧抽象（即：“最后一公里”抽象）提供了一个标准、通用的配置工具。更重要的是，平台工程师或者系统管理员，可以随时随地的每个能力对应的 CUE 模板进行修改，这些修改一旦提交到 Kubernetes，用户在 Appfile 里就可以立刻使用到新的抽象，不需要重新部署或者安装 KubeVela。

在具体实现层，KubeVela 是基于 OAM Kubernetes Runtime 构建的，同时采用 KEDA ，Flagger，Prometheus 等生态项目作为 Trait 的背后的依赖。当然，这些依赖只是 KubeVela 的选型，你可以随时为 KubeVela 定制和安装你喜欢的任何能力作为 Workload Type 或者 Trait。综合以上讲解，KubeVela 项目的整体架构由**用户界面层，模型层，和能力管理层**三部分组成，如下所示：

![](https://tva2.sinaimg.cn/large/ad5fbf65gy1glf9sktkdxj20q00dsacl.jpg)

有了 KubeVela，平台工程师终于拥有了一个可以方便快捷地将任何一个 Kubernetes 社区能力封装抽象成一个面向用户的上层平台特性的强大工具。而作为这个平台的最终用户，应用开发者们只需要学习这些上层抽象，在一个配置文件中描述应用，就可以一键交付出去。

### 3. KubeVela VS 经典 PaaS 

很多人可能会问，KubeVela 跟经典 PaaS 的主要区别和联系是什么呢？

事实上，大多数经典 PaaS 都能提供完整的应用生命周期管理功能，同时也非常关注提供简单友好的用户体验，提升研发效能。在这些点上，KubeVela 跟经典 PaaS 的目标，是非常一致的。

但另一方面，经典 PaaS 往往是不可扩展的（比如 Rancher 的 Rio 项目），或者会引入属于自己的插件生态（哪怕这个 PaaS 是完全基于 Kubernetes 构建的），以此来确保平台本身的用户体验和能力的可控制性（比如 Cloud Foundry 或者 Heroku 的[插件中心](https://elements.heroku.com/addons)）。


相比之下，KubeVela 的设计是完全不同的。KubeVela 的目标，从一开始就是利用整个 Kubernetes 社区作为自己的“插件中心”，并且“故意”把它的每一个内置能力都设计成是独立的、可插拔的插件。这种高度可扩展的模型，背后其实有着精密的设计与实现。比如，KubeVela 如何确保某个完全独立的 Trait 一定能够绑定于某种 Workload Type？如何检查这些相互独立的 Trait 是否冲突？这些挑战，正是 Open Application Model（OAM）作为 KubeVela 模型层的起到的关键作用，一言以蔽之：OAM 是一个高度可扩展的应用定义与能力管理模型。

KubeVela 和 OAM 社区欢迎大家设计和制作任何 Workload Type 和 Trait 的定义文件。只要把它们存放在 GitHub 上，全世界任何一个 KubeVela 用户就都可以在自己的 Appfile 里使用你所设计的能力。具体的方式，请参考 `$ vela cap` （即：插件能力管理命令）的[使用文档](https://kubevela.io/#/en/developers/cap-center)。

## 了解更多

KubeVela 项目是 OAM 社区的官方项目，旨在取代原先的 Rudr 项目。不过，与 [Rudr](https://github.com/oam-dev/rudr) 主要作为“参考实现”的定位不同，KubeVela 既是一个端到端、面向全量场景的 OAM Kubernetes 完整实现，同时也是阿里云 EDAS 服务和内部多个核心 PaaS/Serverless 生产系统底层的核心组件。 此外，KubeVela 中 Apppfile 的设计，也是 OAM 社区在 OAM 规范中即将引入的“面向用户侧对象”的核心部分。

如果你想要更好的了解 KubeVela 项目，欢迎前往其官方网站上[学习具体的示例和手册](https://kubevela.io/)。以下也是一些非常好的学习内容和方式：

- 前往学习 [KubeVela Quick Start（新手教程）](https://kubevela.io/#/en/quick-start)，一步步了解 KubeVela 的使用方法。
- 前往 OAM 社区深入交流和反馈。中文：钉钉群 23310022，英文：[Gitter](https://gitter.im/oam-dev/community) 和 [CNCF Slack](https://cloud-native.slack.com/archives/C01BLQ3HTJA)。
- 尝试为 [KubeVela 添加来自开源社区的插件能力](https://kubevela.io/#/en/platform-engineers/trait)。此外，如果你有任何关于扩展 KubeVela 的奇妙想法，比如，基于 KubeVela 开发一个自己的云原生数据库 PaaS 或者 AI PaaS，欢迎前往 OAM 社区通过 Issue 来进行讨论。
- 为 KubeVela 贡献代码. KubeVela 项目是一个诞生自云原生社区的开源项目（感谢来自 8 家不同公司的[初始贡献者](https://github.com/oam-dev/kubevela/blob/bbb2c527d96d3e1a0694e2f49b3d1d1168e72c53/OWNERS_ALIASES#L35)，并特别鸣谢 KubeVela 网站的发起者 [guoxudong](https://github.com/sunny0826)）。 

**KubeVela 项目的维护者会在项目稳定后，即将整个项目所有权捐赠给中立开源基金会。**