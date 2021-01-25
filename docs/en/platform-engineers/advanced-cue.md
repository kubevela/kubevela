# Using CUE to Extend Trait in advanced way

> WARNINIG: you are now reading a platform builder/administrator oriented documentation.

In the following tutorial, you will learn how to add a trait in a more advanced way without writing any CRD controller.
In general, the advanced way can help you build abstraction by composition or decomposition.

## Trait generate multiple resources

With the help of CUE template, we can combine multiple K8s resources into one trait.
 
You can use the keyword `outputs` to create multiple K8s objects. The format MUST be `outputs:<unique-name>:<k8s-object>`.

Let's look at an example, assume you hope to make a combo for K8s service and ingress, naming it as `ingress`.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  name: ingress
spec:
  extension:
    template: |
      parameter: {
        domain: string
        http: [string]: int
      }

      // trait template can have multiple outputs in one trait
      outputs: service: {
        apiVersion: "v1"
        kind: "Service"
        spec: {
          selector:
            app: context.name
          ports: [
            for k, v in parameter.http {
              port: v
              targetPort: v
            }
          ]
        }
      }

      outputs: ingress: {
        apiVersion: "networking.k8s.io/v1beta1"
        kind: "Ingress"
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
                }
              ]
            }
          }]
        }
      }
```

Apply this newly defined TraitDefinition into our system:

```shell script
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/docs/examples/advanced-cue/newroute.yaml
```

You can check it by using the application object like below:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: Application
metadata:
  name: testapp
spec:
  components:
    - name: express-server
      scopes:
        healthscopes.core.oam.dev: testapp-default-health
      settings:
        cmd:
          - node
          - server.js
        image: oamdev/testapp:v1
        port: 8080
      traits:
        - name: ingress
          properties:
            domain: test.my.domain
            http:
              "/api": 8080
      type: webservice
```

Apply it:

```shell script
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/docs/examples/advanced-cue/app1.yaml
```

Then you will see the deployment behind webservice along with the K8s service and ingress behind the ingress trait created.

### Generate multiple resources by using for loop

You can use a for-loop to generate multiple resources if you want. 
The key point is you should define the for-loop inside the outputs,
the type of parameter field used in the for-loop must be a map. 

Below is an example that will generate multiple K8s services in one trait:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  name: expose
spec:
  extension:
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


Apply this newly defined TraitDefinition into our system:

```shell script
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/docs/examples/advanced-cue/for-loop.yaml
```

Use the newly created trait like below:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: Application
metadata:
  name: testapp
spec:
  components:
    - name: express-server
      type: webservice
      settings:
        cmd:
          - node
          - server.js
        image: oamdev/testapp:v1
        port: 8080
      traits:
        - name: expose
          properties:
            http:
              myservice1: 8080
              myservice2: 8081
```

Apply it:

```shell script
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/docs/examples/advanced-cue/app2.yaml
```

Then you will see the deployment behind webservice along with two K8s services created.


## Patch Trait

For the purpose of separate of concerns, we usually won't do decomposition some fields out as trait from the underlying workload.

For example the [webservice workload] is implemented by K8s Deployment, but the workload doesn't care about the `replicas` field.
In this case, you can write a [ManualScalerTrait](https://github.com/oam-dev/kubevela/tree/master/pkg/controller/core.oam.dev/v1alpha2/core/traits/manualscalertrait)
CRD controller to control the `replicas` field after the deployment created.
 
But now, you are more encouraged to use patch trait in KubeVela. With the help of patch trait, you don't need to write CRD
controller for this case anymore.

The keyword is `patch`, object describe after the keyword will be patched into the workload.
 
Below is an example:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "Manually scale the app"
  name: scaler
spec:
  appliesToWorkloads:
    - webservice
    - worker
  extension:
    template: |-
      patch: {
         spec: replicas: parameter.replicas
      }
      parameter: {
      	replicas: *1 | int
      }
```

The patch trait rely on the workload object will always match the structure `spec.replicas` and the type of the field.
So we usually use it with the field `appliesToWorkloads` which can limit the trait can only be used by these specified workloads.

By default, the patch implemented in KubeVela relies on the CUE merge operation. It has these constraints:

* New field will be added only when the schema doesn't conflict with each other, and the value not finalized.  

For example, if the workload already define the `spec.replicas` is `5`, then the patch trait replicas value `1` will fail to patch.

* Array list in the patch will be merged into the workload by the order of index, you need to use strategy patch.


### Strategy Patch Trait

`strategy patch` is a special patch logic for patching array list supported in KubeVela, it's not native CUElang feature,
so you need to write annotation for using it.

The annotation keyword is `//+patchKey=<key_name>`.

By adding this annotation, merging logic of two array list will not follow the CUE rule, instead of that, it will
regard the element type of the array list will always be object, and compare the object field with the specified key name.
If the value of the key name equal, then the patch data will merge into that, if no equal found, the patch will append into the array list.

The example of strategy patch trait will like below:
 
```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "add sidecar to the app"
  name: sidecar
spec:
  appliesToWorkloads:
    - webservice
    - worker
  extension:
    template: |-
      patch: {
         // +patchKey=name
         spec: template: spec: containers: [parameter]
      }
      parameter: {
         name: string
         image: string
         command?: [...string]
      }
```

The patchKey is `name` which represents the container name in this example. In this case, if the workload already has
a container with the same name of this `sidecar` trait, it will be a merge operation. If the workload don't have the container
with same name, it will be a sidecar container append into the `spec.template.spec.containers` array list.

### Patch works with output

Patch can also work with output, if patch and output both exist in one trait, the patch part will execute first and then 
the output object will be rendered out. 

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "service the app"
  name: kservice
spec:
  appliesToWorkloads:
    - webservice
    - worker
  definitionRef:
    name: services
  extension:
    template: |-
      patch: {spec: template: metadata: labels: app: context.name}
      output: {
        apiVersion: "v1"
        kind: "Service"
        metadata: name: context.name
        spec: {
          selector:  app: context.name
          ports: [
            for k, v in parameter.http {
              port: v
              targetPort: v
            }
          ]
        }
      }
      parameter: {
        http: [string]: int
      }
```

## Processing Trait

A KubeVela trait can also help you to do some processing job. Currently, we have supported http request.

The keyword is `processing`, inside the `processing`, there are two keywords `output` and `http`.

You can define http request `method`, `url`, `body`, `header` and `trailer` in the `http` section.
KubeVela will send a request using this information, the requested server shall output a **json result**.

The `output` section will used to match with the `json result`, correlate fields by name will be automatically filled into it.
Then you can use the requested data from `processing.output` into `patch` or `output/outputs`.

Below is an example:

```yaml
apiVersion: core.oam.dev/v1alpha1
kind: TraitDefinition
metadata:
  name: auth-service
spec:
  template: |
    parameter: {
        serviceURL: string
    }

    processing: {
      output: {
        token?: string
      }
      # task shall output a json result and output will correlate fields by name.
      http: {
        method: *"GET" | string
        url: parameter.serviceURL
        request: {
            body ?: bytes
            header: {}
            trailer: {}
        }
      }
    }

    patch: {
      data: token: processing.output.token
    }
```


## More Useful Trait Use Cases

Patch trait can be powerful, let me show more interesting use cases for you.

### Add labels

When you want to add some common labels into the pod template, for example, use the label as a virtual group.

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
  extension:
    template: |-
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
apiVersion: core.oam.dev/v1alpha2
kind: Application
spec:
  ...
      traits:
        - name: virtualgroup
          properties:
            group: "my-group1"
            type: "cluster"
```

In this example, different type will use different label key.

### Add Annotations

Similar to common labels, you may want to add some information into the controller for some extension.

Below is an example that represents auto scale bound by using annotation. The annotation value will be a json string.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "Specify auto scale by annotation"
  name: kautoscale
spec:
  appliesToWorkloads:
    - webservice
    - worker
  extension:
    template: |-
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

### Add Pod Env

Inject some system environments into pod is also very common use case.

The example could be like below, this case rely on strategy merge patch, so don't forget add `+patchKey=name` like below:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "add env into your pods"
  name: env
spec:
  appliesToWorkloads:
    - webservice
    - worker
  extension:
    template: |-
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

This example put uid token in http header, you can also use request body.
You may refer to [processing](#Processing-Trait) section for more details.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "dynamically specify service account"
  name: service-account
spec:
  appliesToWorkloads:
    - webservice
    - worker
  extension:
    template: |-
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

### Add init container and share volume

A more general way to do some logic before the real business logic is to use init container.
You can define any operations in an image and run it as init container, after that use a shared volume to mount into the pod.

Here is an example for this use case, it's same with the [K8s init container demo](https://kubernetes.io/docs/tasks/configure-pod-container/configure-pod-initialization/#create-a-pod-that-has-an-init-container)

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "add an init container and use shared volume with pod"
  name: init-container
spec:
  appliesToWorkloads:
    - webservice
    - worker
  extension:
    template: |-
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
      			name:    parameter.name
      			image:   parameter.image
      			command: parameter.command
      			// +patchKey=name
      			volumeMounts: [{
      				name:      parameter.mountName
      				mountPath: parameter.initMountPath
      			}]
      		}]
      		// +patchKey=name
      		volumes: [{
      			name:     parameter.mountName
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
apiVersion: core.oam.dev/v1alpha2
kind: Application
metadata:
  name: testapp
spec:
  components:
    - name: express-server
      type: webservice
      settings:
        image: oamdev/testapp:v1
      traits:
        - name: "init-container"
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

### Node affinity and anti-affinity

Node affinity and anti-affinity is also common trait:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "affinity specify node affinity and toleration"
  name: node-affinity
spec:
  appliesToWorkloads:
    - webservice
    - worker
  extension:
    template: |-
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
      settings:
        image: oamdev/testapp:v1
      traits:
        - name: "node-affinity"
          properties:
            affinity:
              server-owner: ["owner1","owner2"]
              resource-pool: ["pool1","pool2","pool3"]
            tolerations:
              resource-pool: "broken-pool1"
              server-owner: "old-owner"
```