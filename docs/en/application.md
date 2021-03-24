# Using `Application` to Describe Your App

This documentation will walk through how to use `Application` object to define your apps with corresponding operational behaviors in declarative approach.

## Example

The sample application below claimed a `backend` component with *Worker* workload type, and a `frontend` component with *Web Service* workload type.

Moreover, the `frontend` component claimed `sidecar` and `autoscaler` traits which means the workload will be automatically injected with a `fluentd` sidecar and scale from 1-100 replicas triggered by CPU usage.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: Application
metadata:
  name: website
spec:
  components:
    - name: backend
      type: worker
      settings:
        image: busybox
        cmd:
          - sleep
          - '1000'
    - name: frontend
      type: webservice
      settings:
        image: nginx
      traits:
        - name: autoscaler
          properties:
            min: 1
            max: 10
            cpuPercent: 60
        - name: sidecar
          properties:
            name: "sidecar-test"
            image: "fluentd"
```

The `type: worker` means the specification of this workload (claimed in following `settings` section) will be enforced by a `WorkloadDefinition` object named `worker` as below:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: worker
  annotations:
    definition.oam.dev/description: "Describes long-running, scalable, containerized services that running at backend. They do NOT have network endpoint to receive external network traffic."
spec:
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
        				}]
        			}
        		}
        	}
        }
        parameter: {
        	image: string
        	cmd?: [...string]
        }
```


Hence, the `settings` section of `backend` only supports two parameters: `image` and `cmd`, this is enforced by the `parameter` list of the `.spec.template` field of the definition.

The similar extensible abstraction mechanism also applies to traits. For example, `name: autoscaler` in `frontend` means its trait specification (i.e. `properties` section) will be enforced by a `TraitDefinition` object named `autoscaler` as below:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "configure k8s HPA for Deployment"
  name: hpa
spec:
  appliesToWorkloads:
    - webservice
    - worker
  schematic:
    cue:
      template: |
        outputs: hpa: {
        	apiVersion: "autoscaling/v2beta2"
        	kind:       "HorizontalPodAutoscaler"
        	metadata: name: context.name
        	spec: {
        		scaleTargetRef: {
        			apiVersion: "apps/v1"
        			kind:       "Deployment"
        			name:       context.name
        		}
        		minReplicas: parameter.min
        		maxReplicas: parameter.max
        		metrics: [{
        			type: "Resource"
        			resource: {
        				name: "cpu"
        				target: {
        					type:               "Utilization"
        					averageUtilization: parameter.cpuUtil
        				}
        			}
        		}]
        	}
        }
        parameter: {
        	min:     *1 | int
        	max:     *10 | int
        	cpuUtil: *50 | int
        }
```

All the definition objects are expected to be defined and installed by platform team. The end users will only focus on `Application` resource (either render it by tools or author it manually).

## Conventions and "Standard Contract"

After the `Application` resource is applied to Kubernetes cluster, the KubeVela runtime will generate and manage the underlying resources instances following below "standard contract" and conventions.


| Label  | Description |
| :--: | :---------: | 
|`workload.oam.dev/type=<workload definition name>` | The name of its corresponding `WorkloadDefinition` |
|`trait.oam.dev/type=<trait definition name>` | The name of its corresponding `TraitDefinition` | 
|`app.oam.dev/name=<app name>` | The name of the application it belongs to |
|`app.oam.dev/component=<component name>` | The name of the component it belongs to |
|`trait.oam.dev/resource=<name of trait resource instance>` | The name of trait resource instance |

> TBD: the revision names and labels for resource instances are currently work in progress.

> TBD: a demo for kubectl apply above Application CR and show full detailed underlying resources.
