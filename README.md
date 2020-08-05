# RudrX

RudrX is a command-line tool to use OAM based micro-app engine.

## Develop
Check out [DEVELOPMENT.md](./DEVELOPMENT.md) to see how to develop with RudrX

## Build `rudr` binary
```shell script
$ go build -o /usr/local/bin/rudr cmd/rudrx/main.go
$ chmod +x /usr/local/bin/rudr
```

## RudrX commands

#### help
```shell script
$ rudr -h
✈️  A Micro App Plafrom for Kubernetes.

Usage:
  rudr [flags]
  rudr [command]

Available Commands:
  ManualScaler      Attach ManualScaler trait to an app
  SimpleRollout     Attach SimpleRollout trait to an app
  app:delete        Delete OAM Applications
  app:ls            List applications
  app:status        get status of an application
  containerized:run Run containerized workloads
  deployment:run    Run deployment workloads
  env               List environments
  env:delete        Delete environment
  env:init          Create environments
  env:sw            Switch environments
  help              Help about any command
  init              Initialize RudrX on both client and server
  route             Attach route trait to an app
  traits            List traits
  version           Prints out build version information
  workloads         List workloads
```


#### env
```
$ rudr env:init test --namespace test
Create env succeed, current env is test

$ rudr env test
NAME	NAMESPACE
test	test

$ rudr env
NAME   	NAMESPACE
default	default
test   	test

$ rudr env:sw default
Switch env succeed, current env is default

$ rudr env:delete test
test deleted

$ rudr env:delete default
Error: you can't delete current using default
```

#### workload run
```shell script
$ rudr containerized:run app123 -p 80 --image nginx:1.9.4
Creating AppConfig app123
SUCCEED
```

#### app
```
$ rudr app:ls
NAME       	WORKLOAD             	TRAITS                     	STATUS	CREATED-TIME
app123     	ContainerizedWorkload	app123-manualscaler-trait  	False 	2020-08-05 20:19:03 +0800 CST
poc08032042	ContainerizedWorkload	                           	True  	2020-08-03 20:43:02 +0800 CST
poc1039    	ContainerizedWorkload	poc1039-manualscaler-trait 	False 	2020-08-02 10:39:54 +0800 CST


$ rudr app:status app123
status: "False"
trait:
- apiVersion: core.oam.dev/v1alpha2
  kind: ManualScalerTrait
  metadata:
    creationTimestamp: null
    name: app123-manualscaler-trait
  spec:
    definitionRef:
      name: ""
workload:
  apiVersion: core.oam.dev/v1alpha2
  kind: ContainerizedWorkload
  metadata:
    creationTimestamp: null
    name: app123
  spec:
    definitionRef:
      name: ""


$ rudr app:delete app123
Deleting AppConfig "app123"
DELETE SUCCEED
```

#### WorkloadDefinitions/TratiDefinitions
```shell script
$ rudr traits
NAME                              	ALIAS	DEFINITION                        	APPLIES TO                                                  	STATUS
manualscalertraits.core.oam.dev   	     	manualscalertraits.core.oam.dev   	core.oam.dev/v1alpha2.ContainerizedWorkload                 	-
simplerollouttraits.extend.oam.dev	     	simplerollouttraits.extend.oam.dev	core.oam.dev/v1alpha2.ContainerizedWorkload, deployments....	-

$ rudr workloads
NAME                               	SHORT	DEFINITION
containerizedworkloads.core.oam.dev	     	containerizedworkloads.core.oam.dev
deployments.apps                   	     	deployments.apps
```