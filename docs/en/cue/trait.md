# Defining Traits

In this section we will introduce how to define a Trait with CUE template.

## Composition
 
Defining a *Trait* with CUE template is a bit different from *Workload Type*: a trait MUST use `outputs` keyword instead of `output` in template.

With the help of CUE template, it is very nature to compose multiple Kubernetes resources in one trait.
Similarly, the format MUST be `outputs:<unique-name>:<full template>`.

Below is an example for `ingress` trait.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  name: ingress
spec:
  schematic:
    cue:
      template: |
        parameter: {
        	domain: string
        	http: [string]: int
        }

        // trait template can have multiple outputs in one trait
        outputs: service: {
        	apiVersion: "v1"
        	kind:       "Service"
        	spec: {
        		selector:
        			app: context.name
        		ports: [
        			for k, v in parameter.http {
        				port:       v
        				targetPort: v
        			},
        		]
        	}
        }

        outputs: ingress: {
        	apiVersion: "networking.k8s.io/v1beta1"
        	kind:       "Ingress"
        	metadata:
        		name: context.name
        	spec: {
        		rules: [{
        			host: parameter.domain
        			http: {
        				paths: [
        					for k, v in parameter.http {
        						path: k
        						backend: {
        							serviceName: context.name
        							servicePort: v
        						}
        					},
        				]
        			}
        		}]
        	}
        }
```

It can be used in the application object like below:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: testapp
spec:
  components:
    - name: express-server
      type: webservice
      properties:
        cmd:
          - node
          - server.js
        image: oamdev/testapp:v1
        port: 8080
      traits:
        - type: ingress
          properties:
            domain: test.my.domain
            http:
              "/api": 8080
```

### Generate Multiple Resources with Loop

You can define the for-loop inside the `outputs`, the type of `parameter` field used in the for-loop must be a map. 

Below is an example that will generate multiple Kubernetes Services in one trait:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  name: expose
spec:
  schematic:
    cue:
      template: |
        parameter: {
        	http: [string]: int
        }

        outputs: {
        	for k, v in parameter.http {
        		"\(k)": {
        			apiVersion: "v1"
        			kind:       "Service"
        			spec: {
        				selector:
        					app: context.name
        				ports: [{
        					port:       v
        					targetPort: v
        				}]
        			}
        		}
        	}
        }
```

The usage of this trait could be:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: testapp
spec:
  components:
    - name: express-server
      type: webservice
      properties:
        ...
      traits:
        - type: expose
          properties:
            http:
              myservice1: 8080
              myservice2: 8081
```

## Patch Trait

You could also use keyword `patch` to patch data to the component instance (before the resource applied) and claim this behavior as a trait.
 
Below is an example for `node-affinity` trait:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "affinity specify node affinity and toleration"
  name: node-affinity
spec:
  appliesToWorkloads:
    - webservice
    - worker
  schematic:
    cue:
      template: |
        patch: {
        	spec: template: spec: {
        		if parameter.affinity != _|_ {
        			affinity: nodeAffinity: requiredDuringSchedulingIgnoredDuringExecution: nodeSelectorTerms: [{
        				matchExpressions: [
        					for k, v in parameter.affinity {
        						key:      k
        						operator: "In"
        						values:   v
        					},
        				]}]
        		}
        		if parameter.tolerations != _|_ {
        			tolerations: [
        				for k, v in parameter.tolerations {
        					effect:   "NoSchedule"
        					key:      k
        					operator: "Equal"
        					value:    v
        				}]
        		}
        	}
        }

        parameter: {
        	affinity?: [string]: [...string]
        	tolerations?: [string]: string
        }
```

You can use it like:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: Application
metadata:
  name: testapp
spec:
  components:
    - name: express-server
      type: webservice
      properties:
        image: oamdev/testapp:v1
      traits:
        - type: "node-affinity"
          properties:
            affinity:
              server-owner: ["owner1","owner2"]
              resource-pool: ["pool1","pool2","pool3"]
            tolerations:
              resource-pool: "broken-pool1"
              server-owner: "old-owner"
```

The patch trait above assumes the component instance have `spec.template.spec.affinity` schema. Hence we need to use it with the field `appliesToWorkloads` which can enforce the trait only to be used by these specified workload types.

By default, the patch trait in KubeVela relies on the CUE `merge` operation. It has following known constraints:

* Can not handle conflicts. For example, if a field already has a final value `replicas=5`, then the patch trait will conflict when patches `replicas=1` and fail. It only works when `replica` is not finalized before patch.
* Array list in the patch will be merged following the order of index. It can not handle the duplication of the array list members.


### Strategy Patch Trait

The `strategy patch` is a special patch logic for patching array list. This is supported **only** in KubeVela (i.e. not a standard CUE feature).

In order to make it work, you need to use annotation `//+patchKey=<key_name>` in the template.

With this annotation, merging logic of two array lists will not follow the CUE behavior. Instead, it will treat the list as object and use a strategy merge approach: if the value of the key name equal, then the patch data will merge into that, if no equal found, the patch will append into the array list.

The example of strategy patch trait will like below:
 
```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "add sidecar to the app"
  name: sidecar
spec:
  appliesToWorkloads:
    - webservice
    - worker
  schematic:
    cue:
      template: |
        patch: {
        	// +patchKey=name
        	spec: template: spec: containers: [parameter]
        }
        parameter: {
        	name:  string
        	image: string
        	command?: [...string]
        }
```

The patchKey is `name` which represents the container name in this example. In this case, if the workload already has a container with the same name of this `sidecar` trait, it will be a merge operation. If the workload don't have the container with same name, it will be a sidecar container append into the `spec.template.spec.containers` array list.

### Patch The Trait

If patch and outputs both exist in one trait, the patch part will execute first and then the output object will be rendered out. 

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "service the app"
  name: kservice
spec:
  appliesToWorkloads:
    - webservice
    - worker
  schematic:
    cue:
      template: |
        patch: {spec: template: metadata: labels: app: context.name}
        outputs: service: {
        	apiVersion: "v1"
        	kind:       "Service"
        	metadata: name: context.name
        	spec: {
        		selector: app: context.name
        		ports: [
        			for k, v in parameter.http {
        				port:       v
        				targetPort: v
        			},
        		]
        	}
        }
        parameter: {
        	http: [string]: int
        }
```

## Processing Trait

A trait can also help you to do some processing job. Currently, we have supported http request.

The keyword is `processing`, inside the `processing`, there are two keywords `output` and `http`.

You can define http request `method`, `url`, `body`, `header` and `trailer` in the `http` section.
KubeVela will send a request using this information, the requested server shall output a **json result**.

The `output` section will used to match with the `json result`, correlate fields by name will be automatically filled into it.
Then you can use the requested data from `processing.output` into `patch` or `output/outputs`.

Below is an example:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  name: auth-service
spec:
  schematic:
    cue:
      template: |
        parameter: {
        	serviceURL: string
        }

        processing: {
        	output: {
        		token?: string
        	}
        	// task shall output a json result and output will correlate fields by name.
        	http: {
        		method: *"GET" | string
        		url:    parameter.serviceURL
        		request: {
        			body?: bytes
        			header: {}
        			trailer: {}
        		}
        	}
        }

        patch: {
        	data: token: processing.output.token
        }
```

## Simple data passing

The trait can use the data of workload output and outputs to fill itself.

There are two keywords `output` and `outputs` in the rendering context.
You can use `context.output` refer to the workload object, and use `context.outputs.<xx>` refer to the trait object.
please make sure the trait resource name is unique, or the former data will be covered by the latter one.

Below is an example
1. the main workload object(Deployment) in this example will render into the context.output before rendering traits.
2. the context.outputs.<xx> will keep all these rendered trait data and can be used in the traits after them.

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
        					ports: [{containerPort: parameter.port}]
        					envFrom: [{
        						configMapRef: name: context.name + "game-config"
        					}]
        					if parameter["cmd"] != _|_ {
        						command: parameter.cmd
        					}
        				}]
        			}
        		}
        	}
        }

        outputs: gameconfig: {
        	apiVersion: "v1"
        	kind:       "ConfigMap"
        	metadata: {
        		name: context.name + "game-config"
        	}
        	data: {
        		enemies: parameter.enemies
        		lives:   parameter.lives
        	}
        }

        parameter: {
        	// +usage=Which image would you like to use for your service
        	// +short=i
        	image: string
        	// +usage=Commands to run in the container
        	cmd?: [...string]
        	lives:   string
        	enemies: string
        	port:    int
        }


---
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  name: ingress
spec:
  schematic:
    cue:
      template: |
        parameter: {
        	domain:     string
        	path:       string
        	exposePort: int
        }
        // trait template can have multiple outputs in one trait
        outputs: service: {
        	apiVersion: "v1"
        	kind:       "Service"
        	spec: {
        		selector:
        			app: context.name
        		ports: [{
        			port:       parameter.exposePort
        			targetPort: context.output.spec.template.spec.containers[0].ports[0].containerPort
        		}]
        	}
        }
        outputs: ingress: {
        	apiVersion: "networking.k8s.io/v1beta1"
        	kind:       "Ingress"
        	metadata:
        			name: context.name
        	labels: config: context.outputs.gameconfig.data.enemies
        	spec: {
        		rules: [{
        			host: parameter.domain
        			http: {
        				paths: [{
        					path: parameter.path
        					backend: {
        						serviceName: context.name
        						servicePort: parameter.exposePort
        					}
        				}]
        			}
        		}]
        	}
        }
```

## More Use Cases for Patch Trait

Patch trait could be very powerful, here are some more advanced use cases.

### Add Labels

For example, patch common label (virtual group) to the component workload.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "Add virtual group labels"
  name: virtualgroup
spec:
  appliesToWorkloads:
    - webservice
    - worker
  schematic:
    cue:
      template: |
        patch: {
        	spec: template: {
        		metadata: labels: {
        			if parameter.type == "namespace" {
        				"app.namespace.virtual.group": parameter.group
        			}
        			if parameter.type == "cluster" {
        				"app.cluster.virtual.group": parameter.group
        			}
        		}
        	}
        }
        parameter: {
        	group: *"default" | string
        	type:  *"namespace" | string
        }
```

Then it could be used like:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
spec:
  ...
      traits:
      - type: virtualgroup
        properties:
          group: "my-group1"
          type: "cluster"
```

In this example, different type will use different label key.

### Add Annotations

Similar to common labels, you could also patch the component workload with annotations. The annotation value will be a JSON string.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "Specify auto scale by annotation"
  name: kautoscale
spec:
  appliesToWorkloads:
    - webservice
    - worker
  schematic:
    cue:
      template: |
        import "encoding/json"

        patch: {
        	metadata: annotations: {
        		"my.custom.autoscale.annotation": json.Marshal({
        			"minReplicas": parameter.min
        			"maxReplicas": parameter.max
        		})
        	}
        }
        parameter: {
        	min: *1 | int
        	max: *3 | int
        }
```

### Add Pod ENV

Inject some system environments into pod is also very common use case.

The example could be like below, this case rely on strategy merge patch, so don't forget add `+patchKey=name` like below:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "add env into your pods"
  name: env
spec:
  appliesToWorkloads:
    - webservice
    - worker
  schematic:
    cue:
      template: |
        patch: {
        	spec: template: spec: {
        		// +patchKey=name
        		containers: [{
        			name: context.name
        			// +patchKey=name
        			env: [
        				for k, v in parameter.env {
        					name:  k
        					value: v
        				},
        			]
        		}]
        	}
        }

        parameter: {
        	env: [string]: string
        }
```

### Dynamically Pod Service Account

In this example, the service account was dynamically requested from an authentication service and patched into the service.

This example put uid token in http header, you can also use request body. You may refer to [processing](#Processing-Trait) section for more details.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "dynamically specify service account"
  name: service-account
spec:
  appliesToWorkloads:
    - webservice
    - worker
  schematic:
    cue:
      template: |
        processing: {
        	output: {
        		credentials?: string
        	}
        	http: {
        		method: *"GET" | string
        		url:    parameter.serviceURL
        		request: {
        			header: {
        				"authorization.token": parameter.uidtoken
        			}
        		}
        	}
        }
        patch: {
        	spec: template: spec: serviceAccountName: processing.output.credentials
        }

        parameter: {
        	uidtoken:   string
        	serviceURL: string
        }
```

### Add Init Container

Init container is useful to pre-define operations in an image and run it before app container.

> Please check [Kubernetes documentation](https://kubernetes.io/docs/tasks/configure-pod-container/configure-pod-initialization/#create-a-pod-that-has-an-init-container) for more detail about Init Container.

Below is an example of init container trait:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "add an init container and use shared volume with pod"
  name: init-container
spec:
  appliesToWorkloads:
    - webservice
    - worker
  schematic:
    cue:
      template: |
        patch: {
        	spec: template: spec: {
        		// +patchKey=name
        		containers: [{
        			name: context.name
        			// +patchKey=name
        			volumeMounts: [{
        				name:      parameter.mountName
        				mountPath: parameter.appMountPath
        			}]
        		}]
        		initContainers: [{
        			name:  parameter.name
        			image: parameter.image
        			if parameter.command != _|_ {
        				command: parameter.command
        			}

        			// +patchKey=name
        			volumeMounts: [{
        				name:      parameter.mountName
        				mountPath: parameter.initMountPath
        			}]
        		}]
        		// +patchKey=name
        		volumes: [{
        			name: parameter.mountName
        			emptyDir: {}
        		}]
        	}
        }

        parameter: {
        	name:  string
        	image: string
        	command?: [...string]
        	mountName:     *"workdir" | string
        	appMountPath:  string
        	initMountPath: string
        }
```

This case must rely on the strategy merge patch, for every array list, we add a `// +patchKey=name` annotation to avoid conflict.

The usage could be:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: testapp
spec:
  components:
    - name: express-server
      type: webservice
      properties:
        image: oamdev/testapp:v1
      traits:
        - type: "init-container"
          properties:
            name:  "install-container"
            image: "busybox"
            command:
            - wget
            - "-O"
            - "/work-dir/index.html"
            - http://info.cern.ch
            mountName: "workdir"
            appMountPath:  "/usr/share/nginx/html"
            initMountPath: "/work-dir"
```
