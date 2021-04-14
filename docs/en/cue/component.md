---
title:  How-to
---

In this section, it will introduce how to use [CUE](https://cuelang.org/) to declare app components via `ComponentDefinition`.

> Before reading this part, please make sure you've learned the [Definition CRD](../platform-engineers/definition-and-templates) in KubeVela.

## Declare `ComponentDefinition`

Here is a CUE based `ComponentDefinition` example which provides a abstraction for stateless workload type:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: stateless
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  schematic:
    cue:
      template: |
        parameter: {
        	name:  string
        	image: string
        }
        output: {
        	apiVersion: "apps/v1"
        	kind:       "Deployment"
        	spec: {
        		selector: matchLabels: {
        			"app.oam.dev/component": parameter.name
        		}
        		template: {
        			metadata: labels: {
        				"app.oam.dev/component": parameter.name
        			}
        			spec: {
        				containers: [{
        					name:  parameter.name
        					image: parameter.image
        				}]
        			}
        		}
        	}
        }
```
In detail:
- `.spec.workload` is required to indicate the workload type of this component.
- `.spec.schematic.cue.template` is a CUE template, specifically:
    * The `output` filed defines the template for the abstraction.
    * The `parameter` filed defines the template parameters, i.e. the configurable properties exposed in the `Application`abstraction (and JSON schema will be automatically generated based on them).

Let's declare another component named `task`, i.e. an abstraction for run-to-completion workload.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: task
  annotations:
    definition.oam.dev/description: "Describes jobs that run code or a script to completion."
spec:
  workload:
    definition:
      apiVersion: batch/v1
      kind: Job
  schematic:
    cue:
      template: |
        output: {
          apiVersion: "batch/v1"
          kind:       "Job"
          spec: {
            parallelism: parameter.count
            completions: parameter.count
            template: spec: {
              restartPolicy: parameter.restart
              containers: [{
                image: parameter.image
                if parameter["cmd"] != _|_ {
                  command: parameter.cmd
                }
              }]
            }
          }
        }
        parameter: {
          count:   *1 | int
          image:   string
          restart: *"Never" | string
          cmd?: [...string]
        }
```

Save above `ComponentDefintion` objects to files and install them to your Kubernetes cluster by `$ kubectl apply -f stateless-def.yaml task-def.yaml`

## Declare an `Application`

The `ComponentDefinition` can be instantiated in `Application` abstraction as below:

  ```yaml
  apiVersion: core.oam.dev/v1alpha2
  kind: Application
  metadata:
    name: website
  spec:
    components:
      - name: hello
        type: stateless
        properties:
          image: crccheck/hello-world
          name: mysvc
      - name: countdown
        type: task
        properties:
          image: centos:7
          cmd:
            - "bin/bash"
            - "-c"
            - "for i in 9 8 7 6 5 4 3 2 1 ; do echo $i ; done"
  ```

### Under The Hood
<details>

Above application resource will generate and manage following Kubernetes resources in your target cluster based on the `output` in CUE template and user input in `Application` properties.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: backend
  ... # skip tons of metadata info
spec:
  template:
    spec:
      containers:
        - name: mysvc
          image: crccheck/hello-world
    metadata:
      labels:
        app.oam.dev/component: mysvc
  selector:
    matchLabels:
      app.oam.dev/component: mysvc
---
apiVersion: batch/v1
kind: Job
metadata:
  name: countdown
  ... # skip tons of metadata info
spec:
  parallelism: 1
  completions: 1
  template:
    metadata:
      name: countdown
    spec:
      containers:
        - name: countdown
          image: 'centos:7'
          command:
            - bin/bash
            - '-c'
            - for i in 9 8 7 6 5 4 3 2 1 ; do echo $i ; done
      restartPolicy: Never
```  
</details>

## CUE `Context`

KubeVela allows you to reference the runtime information of your application via `conext` keyword.

The most widely used context is application name(`context.appName`) component name(`context.name`).

```cue
context: {
  appName: string
  name: string
}
```

For example, let's say you want to use the component name filled in by users as the container name in the workload instance:

```cue
parameter: {
    image: string
}
output: {
  ...
    spec: {
        containers: [{
            name:  context.name
            image: parameter.image
        }]
    }
  ...
}
```

> Note that `context` information are auto-injected before resources are applied to target cluster.

### Full available information in CUE `context`

| Context Variable  | Description |
| :--: | :---------: |
| `context.appRevision` | The revision of the application |
| `context.appRevisionNum` | The revision number(`int` type) of the application, e.g., `context.appRevisionNum` will be `1` if `context.appRevision` is `app-v1`|
| `context.appName` | The name of the application |
| `context.name` | The name of the component of the application |
| `context.namespace` | The namespace of the application |
| `context.output` | The rendered workload API resource of the component, this usually used in trait |
| `context.outputs.<resourceName>` | The rendered trait API resource of the component, this usually used in trait |


## Composition

It's common that a component definition is composed by multiple API resources, for example, a `webserver` component that is composed by a Deployment and a Service. CUE is a great solution to achieve this in simplified primitives.

> Another approach to do composition in KubeVela of course is [using Helm](/docs/helm/component).

## How-to

KubeVela requires you to define the template of workload type in `output` section, and leave all the other resource templates in `outputs` section with format as below:

```cue
outputs: <unique-name>: 
  <full template data>
```

> The reason for this requirement is KubeVela needs to know it is currently rendering a workload so it could do some "magic" like patching annotations/labels or other data during it.

Below is the example for `webserver` definition: 

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: webserver
  annotations:
    definition.oam.dev/description: "webserver is a combo of Deployment + Service"
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  schematic:
    cue:
      template: |
        output: {
            apiVersion: "apps/v1"
            kind:       "Deployment"
            spec: {
                selector: matchLabels: {
                    "app.oam.dev/component": context.name
                }
                template: {
                    metadata: labels: {
                        "app.oam.dev/component": context.name
                    }
                    spec: {
                        containers: [{
                            name:  context.name
                            image: parameter.image

                            if parameter["cmd"] != _|_ {
                                command: parameter.cmd
                            }

                            if parameter["env"] != _|_ {
                                env: parameter.env
                            }

                            if context["config"] != _|_ {
                                env: context.config
                            }

                            ports: [{
                                containerPort: parameter.port
                            }]

                            if parameter["cpu"] != _|_ {
                                resources: {
                                    limits:
                                        cpu: parameter.cpu
                                    requests:
                                        cpu: parameter.cpu
                                }
                            }
                        }]
                }
                }
            }
        }
        // an extra template
        outputs: service: {
            apiVersion: "v1"
            kind:       "Service"
            spec: {
                selector: {
                    "app.oam.dev/component": context.name
                }
                ports: [
                    {
                        port:       parameter.port
                        targetPort: parameter.port
                    },
                ]
            }
        }
        parameter: {
            image: string
            cmd?: [...string]
            port: *80 | int
            env?: [...{
                name:   string
                value?: string
                valueFrom?: {
                    secretKeyRef: {
                        name: string
                        key:  string
                    }
                }
            }]
            cpu?: string
        }
```

The user could now declare an `Application` with it:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: webserver-demo
  namespace: default
spec:
  components:
    - name: hello-world
      type: webserver
      properties:
        image: crccheck/hello-world
        port: 8000
        env:
        - name: "foo"
          value: "bar"
        cpu: "100m"
```

It will generate and manage below API resources in target cluster:

```shell
$ kubectl get deployment
NAME             READY   UP-TO-DATE   AVAILABLE   AGE
hello-world-v1   1/1     1            1           15s

$ kubectl get svc
NAME                           TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)    AGE
hello-world-trait-7bdcff98f7   ClusterIP   <your ip>       <none>        8000/TCP   32s
```

## What's Next

Please check the [Learning CUE](./basic) documentation about why we support CUE as first-class templating solution and more details about using CUE efficiently.