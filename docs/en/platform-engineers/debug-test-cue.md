# Debug, Test and Dry-run

With flexibility in defining abstractions, it's important to be able to debug, test and dry-run the CUE based definitions. This tutorial will show this step by step.

## Prerequisites

Please make sure below CLIs are present in your environment:
* [`cue` >=v0.2.2](https://cuelang.org/docs/install/)
* [`vela` (>v1.0.0)](https://kubevela.io/#/en/install?id=_3-optional-get-kubevela-cli)


## Define Definition and Template

We recommend to define the `Definition Object` in two separate parts: the CRD part and the CUE template. This enable us to debug, test and dry-run the CUE template.

Let's name the CRD part as `def.yaml`.

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

And the CUE template part as `def.cue`, then we can use CUE commands such as `cue fmt` / `cue vet`  to format and validate the CUE file.

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

After everything is done, there's a script [`hack/vela-templates/mergedef.sh`](https://github.com/oam-dev/kubevela/blob/master/hack/vela-templates/mergedef.sh) to merge the `def.yaml` and `def.cue` into a completed Definition Object.

```shell
$ ./hack/vela-templates/mergedef.sh def.yaml def.cue > microservice-def.yaml
```

## Debug CUE template

### Use `cue vet` to Validate

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

The `reference "context" not found` is a common error in this step as [`context`](en/cue/component?id=cue-context) is a runtime information that only exist in KubeVela controllers. In order to validate the CUE template end-to-end, we can add a mock `context` in `def.cue`.

> Note that you need to remove all mock data when you finished the validation.

```CUE
... // existing template data
context: {
    name: string
}
```

Then execute the command:

```shell
$ cue vet def.cue
some instances are incomplete; use the -c flag to show errors or suppress this message
```

The `reference "context" not found` error is gone, but  `cue vet` only validates the data type which is not enough to ensure the login in template is correct. Hence we need to use `cue vet -c` for complete validation:

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

It now complains some runtime data is incomplete (because `context` and `parameter` do not have value), let's now fill in more mock data in the `def.cue` file:

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

It won't complain now which means validation is passed:

```shell
cue vet def.cue -c
```

#### Use `cue export` to Check the Rendered Resources

The `cue export` can export rendered result in YAMl foramt:

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


## Dry-Run the `Application`

When CUE template is good, we can use `vela system dry-run` to dry run and check the rendered resources in real Kubernetes cluster. This command will exactly execute the same render logic in KubeVela's `Application` Controller adn output the result for you.

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

### Import `kube` Package

KubeVela automatically generates internal CUE packages for all built-in Kubernetes API resources, so you can import them in CUE template. This could simplify how you write the template because some default values are already there, and the imported package will help you validate the template.

Let's try to define a template with help of `kube` package:

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

And dry run to see the rendered resources:

```shell
vela system dry-run -f test-app.yaml -d componentdef.yaml
```
