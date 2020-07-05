# RudrX
RudrX is a micro-app engine core for k8s based on OAM in a Cli style.

# Getting started
## build the Cli binary
```shell script
$ go build -o rudr
```

# Notes
- Support Component/ApplicationConfiguration manifest generation by `rudr run containerized` command, like
`rudr run containerized frontend -p 80 nginx:1.9.4`

```shell script
$ rudr run containerized frontend -p 80 nginx:1.9.4
Successfully created manifests for Component and ApplicationConfiguration.

$ ll ./.rudr/.build/frontend
total 16
-rw-r--r--  1 zhouzhengxi  staff   177B Jul  5 23:55 appconfig-frontend.yaml
-rw-r--r--  1 zhouzhengxi  staff   428B Jul  5 23:55 component-frontend.yaml

$ kubectl apply -f ./.rudr/.build/frontend
applicationconfiguration.core.oam.dev/frontend unchanged
component.core.oam.dev/frontend configured

$ kubectl get applicationconfiguration
NAME       AGE
frontend   23m
poc        10m

$ kubectl get component
NAME       WORKLOAD-KIND
frontend   ContainerizedWorkload
poc        ContainerizedWorkload

$ kubectl get ContainerizedWorkload
NAME       AGE
frontend   23m
poc        10m
```

- Cli help and warning prompts on `rudr run`
```shell script
$ rudr
rudi is the cli for RudrX, which is a micro-app engine core for k8s based on OAM.

Usage:
  rudr [command]

Available Commands:
  help        Help about any command
  run         Create an OAM component, or ...
Example: rudr run containerized frontend -p 80 oam-dev/demo:v1

Flags:
  -h, --help   help for rudr

Use "rudr [command] --help" for more information about a command.

$ rudr run
You must specify a Component workload, like `ContainerizedWorkload`, or ...

error: Required OAM Component workload not specified.
See 'rudr run -h' for help and examples

$ rudr run containerized
You must specify a name for ContainerizedWorkload

error: Required the name for OAM Component workload ContainerizedWorkload
See 'rudr run -h' for help and examples

$ rudr run containerized front
You must specify a name for ContainerizedWorkload, or image and its tag for ContainerizedWorkload

error: Required ContainerizedWorkload name or image for OAM Component workload ContainerizedWorkload, like nginx:1.9.4
See 'rudr run -h' for help and examples

$ rudr run containerized front nginx:1.9.4
Successfully created manifests for Component and ApplicationConfiguration.

$ rudr run containerized front -p 80
You must specify a name for ContainerizedWorkload, or image and its tag for ContainerizedWorkload

error: Required ContainerizedWorkload name or image for OAM Component workload ContainerizedWorkload, like nginx:1.9.4
See 'rudr run -h' for help and examples
```

# TODO
- Unit-test for `rudr run`
- E2E test
- Dynamic templates support. 
- `rudr attach`
- `rudr install`
