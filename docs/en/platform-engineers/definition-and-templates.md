# Definition and Templates

This section introduces the basic structure of `Definition` objects including `WorkloadDefinition` and `TraitDefinition`.

---

`WorkloadDefinition` and `TraitDefinition` help user to extend capabilities into KubeVela easily.

In this section, we will tell more about the mechanism of definition object.
If you want to learn how to write Definition files, please refer to:

- [OpenFaaS Workload Type](https://kubevela.io/#/en/platform-engineers/workload-type)
- [RDS(Cloud service) Workload Type](https://kubevela.io/#/en/platform-engineers/cloud-services)
- [KubeWatch Trait](https://kubevela.io/#/en/platform-engineers/trait)

Here are two tutorials that introduce how to write a `Definition` file step by step.

- [Workload Definition](https://github.com/oam-dev/kubevela/blob/master/docs/en/platform-engineers/workload-type.md)
- [Trait Definition](https://github.com/oam-dev/kubevela/blob/master/docs/en/platform-engineers/trait.md)

## Structure of Definition File

We will use a built-in `WorkloadDefinition` as a sample to introduce the basic structure of Definition files.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: webservice
  namespace: default
  annotations:
    definition.oam.dev/description: "Describes long-running, scalable, containerized services that have a stable network endpoint to receive external network traffic from customers."
spec:
  definitionRef:
    name: deployments.apps
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
        parameter: {
        	// +usage=Which image would you like to use for your service
        	// +short=i
        	image: string
        
        	// +usage=Commands to run in the container
        	cmd?: [...string]
        
        	// +usage=Which port do you want customer traffic sent to
        	// +short=p
        	port: *80 | int
        	// +usage=Define arguments by using environment variables
        	env?: [...{
        		// +usage=Environment variable name
        		name: string
        		// +usage=The value of the environment variable
        		value?: string
        		// +usage=Specifies a source the value of this var should come from
        		valueFrom?: {
        			// +usage=Selects a key of a secret in the pod's namespace
        			secretKeyRef: {
        				// +usage=The name of the secret in the pod's namespace to select from
        				name: string
        				// +usage=The key of the secret to select from. Must be a valid secret key
        				key: string
        			}
        		}
        	}]
        	// +usage=Number of CPU units for the service, like `0.5` (0.5 CPU core), `1` (1 CPU core)
        	cpu?: string
        }
        
```

Even the definition file looks like verbose and complex, but it only consists of two parts in essence:

- Definition registration part without extensible fields
- CUE template (used by Appfile) part with extensible fields

## Definition Registration Part

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: webservice
  namespace: default
  annotations:
    definition.oam.dev/description: "Describes long-running, scalable, containerized services that have a stable network endpoint to receive external network traffic from customers."
spec:
  definitionRef:
    name: deployments.apps
```

Besides k8s-style type & object metadata, there are only two lines related to definition registration in the sample.

```yaml
  definitionRef:
    name: deployments.apps
```

`.spec.definitionRef` feild refers to the CRD name behind this `Definition`. 
It conforms to such a format: `<resources>.<api-group>`. 
This is a very k8s-idiomatic way to locate resources through `api-group`, `version` and `kind`, while `kind` maps `resources` in K8s RESTful APIs.

Here are two well-known resources in K8s, `Deployment` and `Ingress`.

| api-group         | kind       | version  | resources   |
|-------------------|------------|----------|-------------|
| apps              | Deployment | v1       | deployments |
| networking.k8s.io | Ingress    | v1       | ingresses   |

Therefore, it becomes very intuitive to write a `Definition` file for users who are familiar with K8s API conventions.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: <definition name>
spec:
  definitionRef:
    name: <resources>.<api-group>
```

`TraitDefinition` is defined in the same way as `WorkloadDefinition`, 

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  name: <definition name>
spec:
  definitionRef:
    name: <resources>.<api-group>
```

For example, this `Definition` registers `Ingress` into KubeVela as a trait capability.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  name:  ingress
spec:
  definitionRef:
    name: ingresses.networking.k8s.io
```

### TraitDefinition Function Fields

By contrast to `WorkloadDefinition`, `TraitDefinition` contains several optional fields permitting users to define model-level functions for Trait.

An overall view of these fields in `Definition` file is show as below.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  name:  ingress
spec:
  definitionRef:
    name: ingresses.networking.k8s.io
  appliesToWorkloads: 
    - deployments.apps
    - webservice
  conflictsWith: 
    - service
  workloadRefPath: spec.wrokloadRef 
```

#### AppliesToWorkloads

`.spec.appliesToWorkloads` field defines the constraints that what kinds of workloads this trait is allowded to apply to.
It accepts an array of string as value. 
Each item in the array refers to one or a group of `Workload Type` to which this trait is allowded to apply.

There are four approaches to denote one or a group of `Workload Type`.

- `WorkloadDefinition` name, e.g., `webservice`, `worker`
- `WorkloadDefinition` definition reference (CRD name), e.g., `deployments.apps`
- Resource group of `WorkloadDefinition` definition reference prefixed with `*.`, e.g., `*.apps`, `*.oam.dev`. This means the trait is allowded to apply to any workloads in this group.
- `*` means this trait is allowded to apply to any workloads

If this field is omitted, it means this trait is allowded to apply to any workloads.

KubeVela will raise an error if a trait is applied to a workload which is NOT included in the `appliesToWorkloads`.


#### ConflictsWith

`.spec.conflictsWith` field defines that constraints that what kinds of traits are conflicting with this trait, if they are applied to the same workload.
It accepts an array of string as value. 
Each item in the array refers to one or a group of `Trait`.

There are four approaches to denote one or a group of `Workload Type`.

- `TraitDefinition` name, e.g., `ingress`
- `TraitDefinition` definition reference (CRD name), e.g., `ingresses.networking.k8s.io`
- Resource group of `TraitDefinition` definition reference prefixed with `*.`, e.g., `*.networking.k8s.io`. This means the trait is conflicting with any traits in this group.
- `*` means this trait is conflicting with any other trait.

If this field is omitted, it means this trait is NOT conflicting with any traits.

#### WorkloadRefPath

`.spec.workloadRefPath` field defines the field path of the trait which is used to store the reference of the workload to which the trait is applied.
It accepts a string as value, e.g., `spec.workloadRef`.  

If this field is assigned a value, KubeVela core will automatically fill the workload reference into target field of the trait. 
Then the trait controller can get the workload reference from the trait latter. 
So this field usually accompanies with the traits whose controllers relying on the workload reference at runtime. 

[Scaler](https://github.com/oam-dev/kubevela/blob/master/charts/vela-core/templates/defwithtemplate/manualscale.yaml), a built-in trait, shows a good practice on this field.

## CUE Template Part

CUE template used by Appfile is defined in `.spec.schematic.cue` field. 
As a big topic and significant characteristic in KubeVela, more details about CUE template and multiple advanced usages are introduced in a series of articles.

- [CUE Basic](/en/cue/basic.md)
- [Workload Type](/en/cue/workload-type.md)
- [Trait](/en/cue/trait.md)
- [Advanced Features](/en/cue/status.md)

