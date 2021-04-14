---
title:  Patch Traits
---

**Patch** is a very common pattern of trait definitions, i.e. the app operators can amend/path attributes to the component instance (normally the workload) to enable certain operational features such as sidecar or node affinity rules (and this should be done **before** the resources applied to target cluster).

This pattern is extremely useful when the component definition is provided by third-party component provider (e.g. software distributor) so app operators do not have privilege to change its template.

> Note that even patch trait itself is defined by CUE, it can patch any component regardless how its schematic is defined (i.e. CUE, Helm, and any other supported schematic approaches).

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
  podDisruptive: true
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

The patch trait above assumes the target component instance have `spec.template.spec.affinity` field.
Hence, we need to use `appliesToWorkloads` to enforce the trait only applies to those workload types have this field.

Another important field is `podDisruptive`, this patch trait will patch to the pod template field,
so changes on any field of this trait will cause the pod to restart, We should add `podDisruptive` and make it to be true
to tell users that applying this trait will cause the pod to restart.


Now the users could declare they want to add node affinity rules to the component instance as below:

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

### Known Limitations

By default, patch trait in KubeVela leverages the CUE `merge` operation. It has following known constraints though:

- Can not handle conflicts.
  - For example, if a component instance already been set with value `replicas=5`, then any patch trait to patch `replicas` field will fail, a.k.a you should not expose `replicas` field in its component definition schematic.
- Array list in the patch will be merged following the order of index. It can not handle the duplication of the array list members. This could be fixed by another feature below.

### Strategy Patch

The `strategy patch` is useful for patching array list.

> Note that this is not a standard CUE feature, KubeVela enhanced CUE in this case.

With `//+patchKey=<key_name>` annotation, merging logic of two array lists will not follow the CUE behavior. Instead, it will treat the list as object and use a strategy merge approach:
 - if a duplicated key is found, the patch data will be merge with the existing values; 
 - if no duplication found, the patch will append into the array list.

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
  podDisruptive: true
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

In above example we defined `patchKey` is `name` which is the parameter key of container name. In this case, if the workload don't have the container with same name, it will be a sidecar container append into the `spec.template.spec.containers` array list. If the workload already has a container with the same name of this `sidecar` trait, then merge operation will happen instead of append (which leads to duplicated containers).

If `patch` and `outputs` both exist in one trait definition, the `patch` operation will be handled first and then render the `outputs`. 

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "expose the app"
  name: expose
spec:
  appliesToWorkloads:
    - webservice
    - worker
  podDisruptive: true
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

So the above trait which attaches a Service to given component instance will patch an corresponding label to the workload first and then render the Service resource based on template in `outputs`.

## More Use Cases of Patch Trait

Patch trait is in general pretty useful to separate operational concerns from the component definition, here are some more examples.

### Add Labels

For example, patch common label (virtual group) to the component instance.

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
  podDisruptive: true
  schematic:
    cue:
      template: |
        patch: {
        	spec: template: {
        		metadata: labels: {
        			if parameter.scope == "namespace" {
        				"app.namespace.virtual.group": parameter.group
        			}
        			if parameter.scope == "cluster" {
        				"app.cluster.virtual.group": parameter.group
        			}
        		}
        	}
        }
        parameter: {
        	group: *"default" | string
        	scope:  *"namespace" | string
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
          scope: "cluster"
```

### Add Annotations

Similar to common labels, you could also patch the component instance with annotations. The annotation value should be a JSON string.

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
  podDisruptive: false
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

### Add Pod Environments

Inject system environments into Pod is also very common use case.

> This case relies on strategy merge patch, so don't forget add `+patchKey=name` as below:

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
  podDisruptive: true
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

### Inject `ServiceAccount` Based on External Auth Service

In this example, the service account was dynamically requested from an authentication service and patched into the service.

This example put UID token in HTTP header but you can also use request body if you prefer.

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
  podDisruptive: true
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

The `processing.http` section is an advanced feature that allow trait definition to send a HTTP request during rendering the resource. Please refer to [Execute HTTP Request in Trait Definition](#Processing-Trait) section for more details.

### Add `InitContainer`

[`InitContainer`](https://kubernetes.io/docs/tasks/configure-pod-container/configure-pod-initialization/#create-a-pod-that-has-an-init-container) is useful to pre-define operations in an image and run it before app container.

Below is an example:

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
  podDisruptive: true
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
