# Development Guide

This document will guide platform builders use `cue` and `vela` tools to test/dry-run your CUE templates. In this guide we will take [GoogleCloudPlatform_MicroServices_Demo](https://github.com/oam-dev/samples/tree/master/7.GoogleCloudPlatform_MicroServices_Demo) as example.

## Create Definition 

We take the workload [microservice](https://github.com/oam-dev/samples/blob/master/7.GoogleCloudPlatform_MicroServices_Demo/Definitions/workloads/microservice.yaml) as example, microservice describe a workload component Deployment with Service.

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

Use script `/hack/vela-templates/mergedef.sh` to merge the `def.yaml` and `def.cue`.(Export the script path to the Env can impove your development efficiency.)

```shell
$ export PATH="$PATH:kubevela_project_path/hack/vela-templates"
$ mergedef.sh def.yaml def.cue > workloaddef.yaml
```


### Test CUE Template

After finish the CUE Template Part, we have 3 methods to verify the correctness of the CUE Template.

#### cue fmt

`cue fmt` 


#### cue vet 

#### cue export 

## Dry-Run Application