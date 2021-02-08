# Workload Type with CUE

In the [CUE basic section](./basic.md), we have explained how CUE works as template of Workload Type and Trait.
In this section, we will introduce more details about workload type.

## Basic Usage

The very basic usage of CUE in workload is to extend a K8s Resource as a workload type(WorkloadDefinition).

A K8s Deployment as a workload:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: worker
spec:
  definitionRef:
    name: deployments.apps
  template: |
    parameter: {
        name: string
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

A K8s Job as workload:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: task
  annotations:
    definition.oam.dev/description: "Describes jobs that run code or a script to completion."
spec:
  definitionRef:
    name: jobs.batch
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
    	count: *1 | int
    	image: string
    	restart: *"Never" | string    
    	cmd?: [...string]
    }   
```

Other resources are the same, you can define all K8s resources include CRD in this way.

## Context

When you want to reference the runtime instance name for an app, you can use the `conext` keyword instead of define a new parameter.

KubeVela will provide a `context` struct including app name(`context.appName`) and component name(`context.name`).

```cue
context: {
  appName: string
  name: string
}
```

Values of the context will be injected automatically when an application is deploying.
So you can reference the context variable to use this information.

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

A workload type can contain multiple K8s resources, for example, a webserver workload type may be composed by
K8s Deployment and Service.

The main workload resource MUST be defined in keyword `output` while the auxiliary workload resources MUST be defined
in keyword `outputs` with a resource name inside.

The format MUST be `outputs:<unique-name>:<k8s-object>`.

In the underlying OAM model, the `output` resource will become the `workload` object while the `outputs` resources will
become traits. 

Below is the example: 

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: webserver
  annotations:
    definition.oam.dev/description: "webserver was composed by deployment and service"
spec:
  definitionRef:
    name: deployments.apps
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
    // workload can have extra object composition by using 'outputs' keyword
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

The main workload inside the `output` keyword is a K8s Deployment. The auxiliary resources inside the `outputs` field
is a K8s service, the resource name in the CUE template is `service` after the `outputs` keyword.

The resource name will also be labeled on the K8s resource when it is deployed. In this example, the K8s Service will have
a label(`trait.oam.dev/resource=service`).

