# Use CUE to extend Component type

In this section, it will introduce how to use CUE to extend your custom component types.

Before reading this part, please make sure you've learned [the definition and template concepts](../platform-engineers/definition-and-templates.md)
and the [basic CUE](./basic.md) knowledge related with KubeVela.

## Write ComponentDefinition

Here is a basic `ComponentDefinition` example:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: mydeploy
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

- `.spec.workload` is required to indicate the workload(apiVersion/Kind) defined in the CUE.
- `.spec.schematic.cue.template` is a CUE template, it defines two keywords for KubeVela to build the application abstraction:
    * The `parameter` defines the input parameters from end user, i.e. the configurable fields in the abstraction.
    * The `output` defines the template for the abstraction.


## Create an Application using the CUE based ComponentDefinition

As long as you installed the ComponentDefinition object (e.g. `kubectl apply -f mydeploy.yaml`) with above template into
the K8s system, it can be used like below:

  ```yaml
  apiVersion: core.oam.dev/v1alpha2
  kind: Application
  metadata:
    name: website
  spec:
    components:
      - name: backend
        type: mydeploy
        properties:
          image: crccheck/hello-world
          name: mysvc
  ```

It will finally render out the following object into the K8s system.

```yaml
apiVersion: apps/v1
kind: Deployment
meadata:
  name: mydeploy
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
```  

All information was rendered from the `output` keyword in CUE template.

And so on, a K8s Job as component type could be:

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

## Context in CUE

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

Register the new workload to kubevela. And create an Application to use it:

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

You will finally got the following resources:

```shell
$ kubectl get deployment
NAME             READY   UP-TO-DATE   AVAILABLE   AGE
hello-world-v1   1/1     1            1           15s

$ kubectl get svc
NAME                           TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)    AGE
hello-world-trait-7bdcff98f7   ClusterIP   <your ip>       <none>        8000/TCP   32s
```


## Extend CRD Operator as Component Type

Let's use [OpenKruise](https://github.com/openkruise/kruise) as example of extend CRD as KubeVela Component.
**The mechanism works for all CRD Operators**.

### Step 1: Install the CRD controller

You need to [install the CRD controller](https://github.com/openkruise/kruise#quick-start) into your K8s system.

### Step 2: Create Component Definition

To register Cloneset(one of the OpenKruise workloads) as a new workload type in KubeVela, the only thing needed is to create an `ComponentDefinition` object for it.
A full example can be found in this [cloneset.yaml](https://github.com/oam-dev/catalog/blob/master/registry/cloneset.yaml).
Several highlights are list below.

#### 1. Describe The Workload Type

```yaml
...
  annotations:
    definition.oam.dev/description: "OpenKruise cloneset"
...
```

A one line description of this component type. It will be shown in helper commands such as `$ vela components`.

#### 2. Register it's underlying CRD

```yaml
...
workload:
  definition:
    apiVersion: apps.kruise.io/v1alpha1
    kind: CloneSet
...
```

This is how you register OpenKruise Cloneset's API resource (`fapps.kruise.io/v1alpha1.CloneSet`) as the workload type.
KubeVela uses Kubernetes API resource discovery mechanism to manage all registered capabilities.

#### 4. Define Template

```yaml
...
schematic:
  cue:
    template: |
      output: {
          apiVersion: "apps.kruise.io/v1alpha1"
          kind:       "CloneSet"
          metadata: labels: {
            "app.oam.dev/component": context.name
          }
          spec: {
              replicas: parameter.replicas
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
                    }]
                  }
              }
          }
      }
      parameter: {
          // +usage=Which image would you like to use for your service
          // +short=i
          image: string

          // +usage=Number of pods in the cloneset
          replicas: *5 | int
      }
 ```

### Step 3: Register New Component Type to KubeVela

As long as the definition file is ready, you just need to apply it to Kubernetes.

```bash
$ kubectl apply -f https://raw.githubusercontent.com/oam-dev/catalog/master/registry/cloneset.yaml
```

And the new component type will immediately become available for developers to use in KubeVela.


## A Full Workflow of how to Debug and Test CUE definitions.

This section will explain how to test and debug CUE templates using CUE CLI as well as
dry-run your capability definitions via KubeVela CLI.

### Combine Definition File

Usually we define the Definition file in two parts, one is the yaml part and the other is the CUE part.

Let's name the yaml part as `def.yaml`.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: microservice
  annotations:
    definition.oam.dev/description: "Describes a microservice combo Deployment with Service."
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  schematic:
    cue:
      template: |
```

And the CUE Template part as `def.cue`, then we can use `cue fmt` / `cue vet`  to format and validate the CUE file.

```
output: {
	// Deployment
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: {
		name:      context.name
		namespace: "default"
	}
	spec: {
		selector: matchLabels: {
			"app": context.name
		}
		template: {
			metadata: {
				labels: {
					"app":     context.name
					"version": parameter.version
				}
			}
			spec: {
				serviceAccountName:            "default"
				terminationGracePeriodSeconds: parameter.podShutdownGraceSeconds
				containers: [{
					name:  context.name
					image: parameter.image
					ports: [{
						if parameter.containerPort != _|_ {
							containerPort: parameter.containerPort
						}
						if parameter.containerPort == _|_ {
							containerPort: parameter.servicePort
						}
					}]
					if parameter.env != _|_ {
						env: [
							for k, v in parameter.env {
								name:  k
								value: v
							},
						]
					}
					resources: {
						requests: {
							if parameter.cpu != _|_ {
								cpu: parameter.cpu
							}
							if parameter.memory != _|_ {
								memory: parameter.memory
							}
						}
					}
				}]
			}
		}
	}
}
// Service
outputs: service: {
	apiVersion: "v1"
	kind:       "Service"
	metadata: {
		name: context.name
		labels: {
			"app": context.name
		}
	}
	spec: {
		type: "ClusterIP"
		selector: {
			"app": context.name
		}
		ports: [{
			port: parameter.servicePort
			if parameter.containerPort != _|_ {
				targetPort: parameter.containerPort
			}
			if parameter.containerPort == _|_ {
				targetPort: parameter.servicePort
			}
		}]
	}
}
parameter: {
	version:        *"v1" | string
	image:          string
	servicePort:    int
	containerPort?: int
	// +usage=Optional duration in seconds the pod needs to terminate gracefully
	podShutdownGraceSeconds: *30 | int
	env: [string]: string
	cpu?:    string
	memory?: string
}
```

And finally there's a script [`hack/vela-templates/mergedef.sh`](https://github.com/oam-dev/kubevela/blob/master/hack/vela-templates/mergedef.sh)
can merge the `def.yaml` and `def.cue` to a completed Definition.

```shell
$ ./hack/vela-templates/mergedef.sh def.yaml def.cue > componentdef.yaml
```

### Debug CUE template

#### use `cue vet` to validate

The `cue vet` validates CUE files well.

```shell
$ cue vet def.cue
output.metadata.name: reference "context" not found:
    ./def.cue:6:14
output.spec.selector.matchLabels.app: reference "context" not found:
    ./def.cue:11:11
output.spec.template.metadata.labels.app: reference "context" not found:
    ./def.cue:16:17
output.spec.template.spec.containers.name: reference "context" not found:
    ./def.cue:24:13
outputs.service.metadata.name: reference "context" not found:
    ./def.cue:62:9
outputs.service.metadata.labels.app: reference "context" not found:
    ./def.cue:64:11
outputs.service.spec.selector.app: reference "context" not found:
    ./def.cue:70:11
```

The `reference "context" not found` is a very common error, this is because the [`context`](workload-type.md#context) is
a KubeVela inner variable that will be existed in runtime.

But in order to check the correctness of the CUE Template more conveniently. We can add a fake `context` in `def.cue` for test.

Note that you need to remove it when you have finished the development and test.

```CUE
output: {
	// Deployment
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: {
		name:      context.name
		namespace: "default"
	}
	spec: {
		selector: matchLabels: {
			"app": context.name
		}
		template: {
			metadata: {
				labels: {
					"app":     context.name
					"version": parameter.version
				}
			}
			spec: {
				serviceAccountName:            "default"
				terminationGracePeriodSeconds: parameter.podShutdownGraceSeconds
				containers: [{
					name:  context.name
					image: parameter.image
                    ...
				}]
			}
		}
	}
}
// Service
outputs: service: {
	apiVersion: "v1"
	kind:       "Service"
	metadata: {
		name: context.name
		labels: {
			"app": context.name
		}
	}
	spec: {
		type: "ClusterIP"
		selector: {
			"app": context.name
		}
        ...
	}
}
parameter: {
	version:        *"v1" | string
	image:          string
	servicePort:    int
	containerPort?: int
	// +usage=Optional duration in seconds the pod needs to terminate gracefully
	podShutdownGraceSeconds: *30 | int
	env: [string]: string
	cpu?:    string
	memory?: string
}
context: {
    name: string
}
```

Then execute the command:

```shell
$ cue vet def.cue
some instances are incomplete; use the -c flag to show errors or suppress this message
```

`cue vet` will only validates the data type. The `-c` validates that all regular fields are concrete.
We can fill in the concrete data to verify the correctness of the template.

```shell
$ cue vet def.cue -c
context.name: incomplete value string
output.metadata.name: incomplete value string
output.spec.selector.matchLabels.app: incomplete value string
output.spec.template.metadata.labels.app: incomplete value string
output.spec.template.spec.containers.0.image: incomplete value string
output.spec.template.spec.containers.0.name: incomplete value string
output.spec.template.spec.containers.0.ports.0.containerPort: incomplete value int
outputs.service.metadata.labels.app: incomplete value string
outputs.service.metadata.name: incomplete value string
outputs.service.spec.ports.0.port: incomplete value int
outputs.service.spec.ports.0.targetPort: incomplete value int
outputs.service.spec.selector.app: incomplete value string
parameter.image: incomplete value string
parameter.servicePort: incomplete value int
```

Again, use the mock data for the `context` and `parameter`, append these following data in your `def.cue` file.

```CUE
context: {
	name: "test-app"
}
parameter: {
	version:       "v2"
	image:         "image-address"
	servicePort:   80
	containerPort: 8000
	env: {"PORT": "8000"}
	cpu:    "500m"
	memory: "128Mi"
}
```

The `cue` will verify the field type in the mock parameter.
You can try any data you want until the following command is executed without complains.

```shell
cue vet def.cue -c
```

#### use `cue export` to check the result

`cue export` can export the result in yaml. It's help you to check the correctness of template with the specified output result.

```shell
$ cue export -e output def.cue --out yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-app
  namespace: default
spec:
  selector:
    matchLabels:
      app: test-app
  template:
    metadata:
      labels:
        app: test-app
        version: v2
    spec:
      serviceAccountName: default
      terminationGracePeriodSeconds: 30
      containers:
        - name: test-app
          image: image-address
```

```shell
$ cue export -e outputs.service def.cue --out yaml
apiVersion: v1
kind: Service
metadata:
  name: test-app
  labels:
    app: test-app
spec:
  selector:
    app: test-app
  type: ClusterIP
```


## Dry-Run Application

After we test the CUE Template well, we can use `vela system dry-run` to dry run an application and test in in real K8s environment.
This command will show you the real k8s resources that will be created.

First, we need use `mergedef.sh` to merge the definition and cue files.

```shell
$ mergedef.sh def.yaml def.cue > componentdef.yaml
```

Then, let's create an Application named `test-app.yaml`.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: boutique
  namespace: default
spec:
  components:
    - name: frontend
      type: microservice
      properties:
        image: registry.cn-hangzhou.aliyuncs.com/vela-samples/frontend:v0.2.2
        servicePort: 80
        containerPort: 8080
        env:
          PORT: "8080"
        cpu: "100m"
        memory: "64Mi"
```

Dry run the application by using `vela system dry-run`.

```shell
$ vela system dry-run -f test-app.yaml -d componentdef.yaml
---
# Application(boutique) -- Comopnent(frontend)
---

apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app.oam.dev/component: frontend
    app.oam.dev/name: boutique
    workload.oam.dev/type: microservice
  name: frontend
  namespace: default
spec:
  selector:
    matchLabels:
      app: frontend
  template:
    metadata:
      labels:
        app: frontend
        version: v1
    spec:
      containers:
      - env:
        - name: PORT
          value: "8080"
        image: registry.cn-hangzhou.aliyuncs.com/vela-samples/frontend:v0.2.2
        name: frontend
        ports:
        - containerPort: 8080
        resources:
          requests:
            cpu: 100m
            memory: 64Mi
      serviceAccountName: default
      terminationGracePeriodSeconds: 30

---
apiVersion: v1
kind: Service
metadata:
  labels:
    app: frontend
    app.oam.dev/component: frontend
    app.oam.dev/name: boutique
    trait.oam.dev/resource: service
    trait.oam.dev/type: AuxiliaryWorkload
  name: frontend
spec:
  ports:
  - port: 80
    targetPort: 8080
  selector:
    app: frontend
  type: ClusterIP

---
```

> Note: `vela system dry-run` will execute the same logic of `Application` controller in KubeVela.
> Hence it's helpful for you to test or debug.

### Import Kube Package

KubeVela automatically generates internal packages for all built-in K8s API resources based on K8s OpenAPI.

With the help of `vela system dry-run`, you can use the `import kube package` feature and test it locally.

So some default values in our `def.cue` can be simplified, and the imported package will help you validate the template:

```cue
import (
   apps "kube/apps/v1"
   corev1 "kube/v1"
)

// output is validated by Deployment.
output: apps.#Deployment
output: {
	metadata: {
		name:      context.name
		namespace: "default"
	}
	spec: {
		selector: matchLabels: {
			"app": context.name
		}
		template: {
			metadata: {
				labels: {
					"app":     context.name
					"version": parameter.version
				}
			}
			spec: {
				terminationGracePeriodSeconds: parameter.podShutdownGraceSeconds
				containers: [{
					name:  context.name
					image: parameter.image
					ports: [{
						if parameter.containerPort != _|_ {
							containerPort: parameter.containerPort
						}
						if parameter.containerPort == _|_ {
							containerPort: parameter.servicePort
						}
					}]
					if parameter.env != _|_ {
						env: [
							for k, v in parameter.env {
								name:  k
								value: v
							},
						]
					}
					resources: {
						requests: {
							if parameter.cpu != _|_ {
								cpu: parameter.cpu
							}
							if parameter.memory != _|_ {
								memory: parameter.memory
							}
						}
					}
				}]
			}
		}
	}
}

outputs:{
  service: corev1.#Service
}


// Service
outputs: service: {
	metadata: {
		name: context.name
		labels: {
			"app": context.name
		}
	}
	spec: {
		//type: "ClusterIP"
		selector: {
			"app": context.name
		}
		ports: [{
			port: parameter.servicePort
			if parameter.containerPort != _|_ {
				targetPort: parameter.containerPort
			}
			if parameter.containerPort == _|_ {
				targetPort: parameter.servicePort
			}
		}]
	}
}
parameter: {
	version:        *"v1" | string
	image:          string
	servicePort:    int
	containerPort?: int
	// +usage=Optional duration in seconds the pod needs to terminate gracefully
	podShutdownGraceSeconds: *30 | int
	env: [string]: string
	cpu?:    string
	memory?: string
}
```

Then merge them.

```shell
mergedef.sh def.yaml def.cue > componentdef.yaml
```

And dry run.

```shell
vela system dry-run -f test-app.yaml -d componentdef.yaml
```