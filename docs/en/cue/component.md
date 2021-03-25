# Defining Components

In this section, we will introduce more examples of using CUE to define component types.

## Basic Usage

The very basic usage of CUE in component abstraction is to extend a Kubernetes resource as a component type(via `ComponentDefinition`) and expose configurable parameters to users.

A Deployment as component type:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: worker
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
        			}}}
        }
```

A Job as workload type:

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

## Context

When you want to reference the runtime instance name for an app, you can use the `conext` keyword to define `parameter`.

KubeVela runtime provides a `context` struct including app name(`context.appName`) and component name(`context.name`).

```cue
context: {
  appName: string
  name: string
}
```

Values of the context will be automatically generated before the underlying resources are applied.
This is why you can reference the context variable as value in the template.

```yaml
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

## Composition

A workload type can contain multiple Kubernetes resources, for example, we can define a `webserver` workload type that is composed by Deployment and Service.

Note that in this case, you MUST define the template of component instance in `output` section, and leave all the other templates in `outputs` with resource name claimed. The format MUST be `outputs:<unique-name>:<full template>`.

> This is how KubeVela know which resource is the running instance of the application component.

Below is the example: 

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

Please save the example as file `webserver.yaml`, then register the new workload to kubevela.

```shell
$ kubectl apply -f webserver.yaml
```

Next, we can use the `webserver` type workload in our application, below is the example:

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
        - name: "PORT"
          value: "8000"
        cpu: "100m"
```

Please save the Application example as file `app.yaml`, then create the new Application.

```shell
kubectl apply -f app.yaml
```

Wait for a while until the status of Application is `running`.

```shell
$ kubectl get application webserver-demo -o yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: webserver-demo
  namespace: default
  ...
spec:
  components:
  - name: hello-world
    type: webserver  
    properties:
      cpu: 100m
      env:
      - name: PORT
        value: "8000"
      image: crccheck/hello-world
      port: 8000
status:
  ...
  services:
  - healthy: true
    name: hello-world
  status: running
```

In the K8s cluster, you will see the following resources are created:

```shell
$ kubectl get deployment
NAME             READY   UP-TO-DATE   AVAILABLE   AGE
hello-world-v1   1/1     1            1           15s

$ kubectl get svc
NAME                           TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)    AGE
hello-world-trait-7bdcff98f7   ClusterIP   <your ip>       <none>        8000/TCP   32s
```

