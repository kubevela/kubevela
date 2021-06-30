---
title:  能力管理
---

在 KubeVela 中，开发者可以从任何包含 OAM 抽象文件的 GitHub 仓库中安装更多的能力（例如：新 component 类型或者 traits ）。我们将这些 GitHub 仓库称为 _Capability Centers_ 。

KubeVela 可以从这些仓库中自动发现 OAM 抽象文件，并且同步这些能力到我们的 KubeVela 平台中。

## 添加能力中心

新增且同步能力中心到 KubeVela：

```bash
$ vela cap center config my-center https://github.com/oam-dev/catalog/tree/master/registry
successfully sync 1/1 from my-center remote center
Successfully configured capability center my-center and sync from remote

$ vela cap center sync my-center
successfully sync 1/1 from my-center remote center
sync finished
```

现在，该能力中心 `my-center` 已经可以使用。

## 列出能力中心

你可以列出或者添加更多能力中心。

```bash
$ vela cap center ls
NAME     	ADDRESS
my-center	https://github.com/oam-dev/catalog/tree/master/registry
```

## [可选] 删除能力中心

删除一个

```bash
$ vela cap center remove my-center
```

## 列出所有可用的能力中心

列出某个中心所有可用的能力。

```bash
$ vela cap ls my-center
NAME               	CENTER   	TYPE               	DEFINITION                    	STATUS     	APPLIES-TO
clonesetservice    	my-center	componentDefinition	clonesets.apps.kruise.io      	uninstalled	[]
```

## 从能力中心安装能力

我们开始从 `my-center` 安装新 component `clonesetservice` 到你的 KubeVela 平台。

你可以先安装 OpenKruise 。

```shell
helm install kruise https://github.com/openkruise/kruise/releases/download/v0.7.0/kruise-chart.tgz
```

从 `my-center` 中安装 `clonesetservice` component 。

```bash
$ vela cap install my-center/clonesetservice
Installing component capability clonesetservice
Successfully installed capability clonesetservice from my-center
```

## 使用新安装的能力

我们先检查 `clonesetservice` component 是否已经被安装到平台：

```bash
$ vela components
NAME           	NAMESPACE  	WORKLOAD                	DESCRIPTION
clonesetservice	vela-system	clonesets.apps.kruise.io	Describes long-running, scalable, containerized services
               	           	                        	that have a stable network endpoint to receive external
               	           	                        	network traffic from customers. If workload type is skipped
               	           	                        	for any service defined in Appfile, it will be defaulted to
               	           	                        	`webservice` type.
```

很棒！现在我们部署使用 Appfile 部署一个应用。

```bash
$ cat << EOF > vela.yaml
name: testapp
services:
  testsvc:
    type: clonesetservice
    image: crccheck/hello-world
    port: 8000
EOF
```

```bash
$ vela up
Parsing vela appfile ...
Load Template ...

Rendering configs for service (testsvc)...
Writing deploy config to (.vela/deploy.yaml)

Applying application ...
Checking if app has been deployed...
App has not been deployed, creating a new deployment...
Updating:  core.oam.dev/v1alpha2, Kind=HealthScope in default
✅ App has been deployed 🚀🚀🚀
    Port forward: vela port-forward testapp
             SSH: vela exec testapp
         Logging: vela logs testapp
      App status: vela status testapp
  Service status: vela status testapp --svc testsvc
```

随后，该 cloneset 已经被部署到你的环境。

```shell
$ kubectl get clonesets.apps.kruise.io
NAME      DESIRED   UPDATED   UPDATED_READY   READY   TOTAL   AGE
testsvc   1         1         1               1       1       46s
```

## 删除能力

> 注意，删除能力前请先确认没有被应用引用。

```bash
$ vela cap uninstall my-center/clonesetservice
Successfully uninstalled capability clonesetservice
```
