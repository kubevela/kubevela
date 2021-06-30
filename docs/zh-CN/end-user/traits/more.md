---
title:  更多用法
---

KubeVela 中的 Traits 被设计为模块化的构建块，它们是完全可定制和可插拔的。

## 1. 从能力中心获取

KubeVela 允许您探索由平台团队维护的功能。 kubectl vela 中有两个命令插件：`comp` 和`trait`。

如果您尚未安装 kubectl vela 插件：请参阅 [这里](../../developers/references/kubectl-plugin#install-kubectl-vela-plugin)。

### 1. 列表

例如，让我们尝试列出注册表中所有可用的 trait：

```shell
$ kubectl vela trait --discover
Showing traits from registry: https://registry.kubevela.net
NAME           	REGISTRY	  DEFINITION                    		APPLIES-TO               
service-account	default  	                              		    [webservice worker]      
env            	default 		                                    [webservice worker]      
flagger-rollout	default       canaries.flagger.app          		[webservice]             
init-container 	default 		                                    [webservice worker]      
keda-scaler    	default       scaledobjects.keda.sh         		[deployments.apps]       
metrics        	default       metricstraits.standard.oam.dev		[webservice backend task]
node-affinity  	default		                              		    [webservice worker]      
route          	default       routes.standard.oam.dev       		[webservice]             
virtualgroup   	default		                              		    [webservice worker] 
```
请注意，`--discover` 标志表示显示不在集群中的所有特征。

### 2.安装

然后你可以安装一个 trait，如：

```shell
$ kubectl vela trait get init-container
Installing component capability init-container
Successfully install trait: init-container                                                                                                 
```

### 3.验证

```shell
$ kubectl get traitdefinition  -n vela-system
NAME             APPLIES-TO                DESCRIPTION
init-container   ["webservice","worker"]   add an init container with a shared volume.
...(other trait definitions)
```

默认情况下，这两个命令将检索功能来自 KubeVela 维护的 [repo](https://registry.kubevela.net)。

## 2. 自己设计

查看 [本文档](../../platform-engineers/cue/trait) 了解如何在
KubeVela 平台。