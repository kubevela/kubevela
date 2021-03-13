# Development Guide

This document will guide platform builders use `cue` and `vela` tools to test/dry-run your CUE templates. In this guide we will take a workloaddefinition in [GoogleCloudPlatform-MicroServices-Demo](https://github.com/oam-dev/samples/tree/master/7.GoogleCloudPlatform_MicroServices_Demo) as example.

## Create Definition 
GoogleCloudPlatform-MicroServices-Demo is a cloud-native microservices demo application. This demo consists of a 10-tier microservices application, we abstracted out a workload type named **[microservice](https://github.com/oam-dev/samples/blob/master/7.GoogleCloudPlatform_MicroServices_Demo/Definitions/workloads/microservice.yaml)**. 
The microservice describes a workload consisting of Deployment and Service. The next steps will take microservice as an example. The specific description file of microservice is as follows:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: microservice
  annotations:
    definition.oam.dev/description: "Describes a microservice combo Deployment with Service."
spec:
  definitionRef:
    name: deployment.apps
  schematic:
    cue:
      template: |
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

This yaml file maybe look too long and complex, while this definition file compose with 2 part:

- Definition registration part without extension fields
- CUE Template section for Appfile

The common development process is divided into 3 steps:

- Splite Definition File: usually we will divide the file into 2 parts(Definition registration and CUE template), so that the platform builder can focus on testing CUE file.
  
- Test CUE Template: after finish the CUE Template, the platform builder needs to verify the correctness of the Template.
  
- Dry-Run Application: when we apply your definition, we can check whether the rendering result meets expectations.

### Splite Definition File

We could splite the definition file to 2 files, registration part can be save as `def.yaml`.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: microservice
  annotations:
    definition.oam.dev/description: "Describes a microservice combo Deployment with Service."
spec:
  definitionRef:
    name: deployment.apps
  schematic:
    cue:
      template: |
```

the CUE Template part can be save as `def.cue`.
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

The script `/hack/vela-templates/mergedef.sh` can merge the `def.yaml` and `def.cue` to a WorkloadDefinition.
Export the script path to the Env can impove your development efficiency.

```shell
$ export PATH="$PATH:your_kubevela_project_path/hack/vela-templates"
$ mergedef.sh def.yaml def.cue > workloaddef.yaml
```


### Test CUE Template

After finish the CUE Template Part, we have 3 methods to verify the correctness of the CUE Template.

#### cue fmt

The `cue fmt` can be used to format your CUE Template, the script `mergedef.sh` will also help you format your CUE Template.

```shell
cue fmt def.cue
```

#### cue vet 

The `cue vet` validates CUE and other data files. By default it will only validate if there are no errors.

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

Some errors will be reported when you execute the command. This is because the `context` is only rendered after the app is submitted.
But in order to check the correctness of the CUE Template more conveniently. We can add a fake `context` in `def.cue`, just used to test.
Note that you need to remove it when you actually use it.

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

`cue vet` will only validates the data type. The -c validates that all regular fields are concrete. In order to verify the correctness of the template rendered by the cue,
we need fill in the concrete data. 

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

So we can mock the concrete data of the `context` and `parameter`.

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

The `cue` will verify the field type in the mock paramter. You can try any data you want until the following command is executed without errors.

```shell
cue vet def.cue -c
```

#### cue export 

`cue export` can export the rendering result. It's help you to check the correctness of template.

```yaml
$ cue export def.cue --out yaml
output:
  apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: string
    namespace: default
  spec:
    selector:
      matchLabels:
        app: string
    template:
      metadata:
        labels:
          app: string
          version: v2
      spec:
        serviceAccountName: default
        terminationGracePeriodSeconds: 30
        containers:
        - name: string
          image: image-address
          ports:
          - containerPort: 8000
          env:
          - name: PORT
            value: "8000"
          resources:
            requests:
              cpu: 500m
              memory: 128Mi
outputs:
  service:
    apiVersion: v1
    kind: Service
    metadata:
      name: string
      labels:
        app: string
    spec:
      type: ClusterIP
      selector:
        app: string
      ports:
      - port: 80
        targetPort: 8000
parameter:
  version: v2
  image: image-address
  servicePort: 80
  containerPort: 8000
  podShutdownGraceSeconds: 30
  env:
    PORT: "8000"
  cpu: 500m
  memory: 128Mi
context:
  name: string
```

## Dry-Run Application

After we test the CUE Template, we can use `vela system dry-run` dry run an application, and output the conversion result to stdout.
First, we need use `mergedef.sh` to format and create the WorkloadDefinition file, then apply the WorkloadDefinition.
Note that you need to remove the mock data before you merge files.

```shell
$ mergedef.sh def.yaml def.cue > workloaddef.yaml
$ kubectl apply -f workloaddef.yaml
```

Next, we save the yaml file as `test-app.yaml` and use `vela system dry-run` to test an Application.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: Application
metadata:
  name: boutique
  namespace: default
spec:
  components:
    - name: frontend
      type: microservice
      settings:
        image: registry.cn-hangzhou.aliyuncs.com/vela-samples/frontend:v0.2.2
        servicePort: 80
        containerPort: 8080
        env:
          PORT: "8080"
        cpu: "100m"
        memory: "64Mi"
```

```shell
$ vela system dry-run -f test-app.yaml
- apiVersion: core.oam.dev/v1alpha2
  kind: ApplicationConfiguration
  metadata:
    creationTimestamp: null
    labels:
      app.oam.dev/name: boutique
    name: boutique
    namespace: default
  spec:
    components:
    - componentName: frontend
      traits:
      - trait:
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
  status:
    dependency: {}
    observedGeneration: 0
- apiVersion: core.oam.dev/v1alpha2
  kind: Component
  metadata:
    creationTimestamp: null
    labels:
      app.oam.dev/name: boutique
    name: frontend
    namespace: default
  spec:
    workload:
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
  status:
    observedGeneration: 0
```