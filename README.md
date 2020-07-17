# RudrX

RudrX is a command-line tool to use OAM based micro-app engine.

## Develop
Check out [DEVELOPMENT.md](./DEVELOPMENT.md) to see how to develop with RudrX

## Use with command-line
### Build `rudr` binary
```shell script
$ cd cmd/rudrx
$ go build -o rudr
$ cp ./rudr /usr/local/bin
```

### RudrX commands

- rudr help/prompts
```shell script
$ rudr -h
rudr is a command-line tool to use OAM based micro-app engine.

Usage:
  rudr [flags]
  rudr [command]

Available Commands:
  bind        Attach a trait to a component
  help        Help about any command
  run         Run OAM workloads
  traits      List traits
```

- create and run an appliction
```shell script
$ rudr run -h
  Create and Run one component one AppConfig OAM APP
  
  Usage:
    rudr run [WORKLOAD_KIND] [args]
    rudr run [command]
  
  Examples:
  
  	rudr run containerized frontend -p 80 oam-dev/demo:v1
  
  
  Available Commands:
    containerized Run containerized workloads
  
  Flags:
    -h, --help          help for run
    -p, --port string

$ rudr run
You must specify a workload, like containerized, deployments.apps, statefulsets.apps

$ rudr run containerized
must specify name for workload

$ go run main.go run containerized poc nginx:1.9.4
Creating AppConfig poc
SUCCEED
```

- list traits
```shell script
$ rudr traits -h
List traits

Usage:
  rudr traits [-workload WORKLOADNAME]

Examples:
rudr traits

Flags:
  -h, --help              help for traits
  -w, --workload string   Workload name

$ rudr traits
  NAME                              	SHORT        	DEFINITION                        	APPLIES TO                                                  	STATUS
  simplerollouttraits.extend.oam.dev	SimpleRollout	simplerollouttraits.extend.oam.dev	core.oam.dev/v1alpha2.ContainerizedWorkload, deployments....	-
```

- apply a trait to the workload
```shell script
$ rudr bind poc ManualScaler --replicaCount 5
Applying trait for component poc
Succeeded!

$ kubectl get applicationconfiguration poc2159 -o yaml
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  creationTimestamp: "2020-07-16T13:58:13Z"
  generation: 2
  name: poc
  namespace: default
  ...
spec:
  components:
  - componentName: poc
    traits:
    - trait:
        apiVersion: core.oam.dev/v1alpha2
        kind: ManualScalerTrait
        metadata:
          name: manualscaler
        spec:
          replicaCount: 5
status:
  conditions:
  - lastTransitionTime: "2020-07-16T13:58:13Z"
    reason: Successfully reconciled resource
    status: "True"
    type: Synced
  workloads:
  - componentName: poc
    traits:
    - traitRef:
        apiVersion: core.oam.dev/v1alpha2
        kind: ManualScalerTrait
        name: manualscaler
    workloadRef:
      apiVersion: core.oam.dev/v1alpha2
      kind: ContainerizedWorkload
      name: poc
```