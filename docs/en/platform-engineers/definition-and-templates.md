# Managing Components and Traits

This documentation explains how to manage available application components and traits in your platform with `WorkloadDefinition` and `TraitDefinition` objects.

Essentially, a definition object in KubeVela is consisted by three section:
- `spec.definitionRef`
  - this is for discovering the provider of this capability.
- `Interoperability Fields`
  - they are for the platform to ensure a trait can work with given workload type. Hence only `TraitDefinition` has these fields.
- `spec.schematic`
  - this defines the encapsulation (i.e. templating and parametering) of this capability. For now, user can choose to use Helm or CUE as encapsulation.

Hence, the basic structure of definition object is as below:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: XxxDefinition
metadata:
  name: <definition name>
spec:
  definitionRef:
    name: <resources>.<api-group>
  schematic:
    cue:
      # cue template ...
    helm:
      # Helm chart ...
  # ... interoperability fields
```

## `spec.definitionRef`

Below is a definition for `Web Service` type in KubeVela: 

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
    ...
        
```

In above example, it claims to leverage Kubernetes Deployment (`deployments.apps`) as the workload type to instantiate this component.

Below is an example of `Ingress` trait:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  name:  ingress
spec:
  definitionRef:
    name: ingresses.networking.k8s.io
    ...
```

Similarly, it claims to leverage Kubernetes Ingress (`ingresses.networking.k8s.io`) as the underlying provider of this capability.

## `Interoperability Fields`

An overall view of these fields in `TraitDefinition` is show as below.

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

Let's explain them in detail.

### `.spec.appliesToWorkloads` 

This field defines the constraints that what kinds of workloads this trait is allowed to apply to.
- It accepts an array of string as value.
- Each item in the array refers to one or a group of workload types to which this trait is allowded to apply.

There are four approaches to denote one or a group of workload types.

- `WorkloadDefinition` name, e.g., `webservice`, `worker`
- `WorkloadDefinition` definition reference (CRD name), e.g., `deployments.apps`
- Resource group of `WorkloadDefinition` definition reference prefixed with `*.`, e.g., `*.apps`, `*.oam.dev`. This means the trait is allowded to apply to any workloads in this group.
- `*` means this trait is allowded to apply to any workloads

If this field is omitted, it means this trait is allowded to apply to any workload types.

KubeVela will raise an error if a trait is applied to a workload which is NOT included in the `appliesToWorkloads`.


### `.spec.conflictsWith` 

This field defines that constraints that what kinds of traits are conflicting with this trait, if they are applied to the same workload.
- It accepts an array of string as value. 
- Each item in the array refers to one or a group of traits.

There are four approaches to denote one or a group of workload types.

- `TraitDefinition` name, e.g., `ingress`
- `TraitDefinition` definition reference (CRD name), e.g., `ingresses.networking.k8s.io`
- Resource group of `TraitDefinition` definition reference prefixed with `*.`, e.g., `*.networking.k8s.io`. This means the trait is conflicting with any traits in this group.
- `*` means this trait is conflicting with any other trait.

If this field is omitted, it means this trait is NOT conflicting with any traits.

### `.spec.workloadRefPath`

This field defines the field path of the trait which is used to store the reference of the workload to which the trait is applied.
- It accepts a string as value, e.g., `spec.workloadRef`.  

If this field is set, KubeVela core will automatically fill the workload reference into target field of the trait. Then the trait controller can get the workload reference from the trait latter. So this field usually accompanies with the traits whose controllers relying on the workload reference at runtime. 

Please check [scaler](https://github.com/oam-dev/kubevela/blob/master/charts/vela-core/templates/defwithtemplate/manualscale.yaml) trait as a demonstration of how to set this field.

## Capability Encapsulation

The encapsulation (i.e. templating and parametering) of given capability are defined in `spec.schematic` field. For example, below is the full definition of `Web Service` type in KubeVela:

<details>

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
</details>

We will explain this section in detail in the following guides:
- [Use CUE](en/cue/basic) to encapsulate capabilities.
- [Use Helm](en/helm/basic) to encapsulate capabilities.


