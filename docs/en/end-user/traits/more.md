---
title:  Want More?
---

Traits in KubeVela are designed as modularized building blocks, they are fully customizable and pluggable.

## 1. Get from capability canter

KubeVela allows you to explore capabilities maintained by platform team. There are two commands in kubectl vela
plugin: `comp` and `trait`.

In case you haven't installed kubectl vela plugin: see [this](../../developers/references/kubectl-plugin#install-kubectl-vela-plugin).

### 1. list

For example, let's try to list all available traits in registry:

```shell
$ kubectl vela trait --discover
Showing traits from registry: https://github.com/oam-dev/catalog/tree/master/registry
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
Note that the `--discover` flag means show all traits not in your cluster.

### 2. install

Then you can install a trait like:

```shell
$ kubectl vela trait get init-container
Installing component capability init-container
Successfully install trait: init-container                                                                                                 
```

### 3.verify

```shell
$ kubectl get traitdefinition  -n vela-system
NAME             APPLIES-TO                DESCRIPTION
init-container   ["webservice","worker"]   add an init container with a shared volume.
...(other trait definitions)
```

By default, the two commands will retrieve capabilities
from [repo](https://github.com/oam-dev/catalog/tree/master/registry) maintained by KubeVela.

## 2. Designed by yourself

Check [this documentation](../../platform-engineers/cue/trait) about how to design and enable your own traits in
KubeVela platform.