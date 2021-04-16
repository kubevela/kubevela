---
title:  KEDA as Autoscaling Trait
---

> Before continue, make sure you have learned about the concepts of [Definition Objects](definition-and-templates) and [Defining Traits with CUE](/docs/cue/trait) section.

In the following tutorial, you will learn to add [KEDA](https://keda.sh/) as a new autoscaling trait to your KubeVela based platform.

> KEDA is a Kubernetes-based Event Driven Autoscaler. With KEDA, you can drive the scaling of any container based on resource metrics or the number of events needing to be processed.

## Step 1: Install KEDA controller

[Install the KEDA controller](https://keda.sh/docs/2.2/deploy/) into your K8s system.

## Step 2: Create Trait Definition

To register KEDA as a new capability (i.e. trait) in KubeVela, the only thing needed is to create an `TraitDefinition` object for it.

A full example can be found in this [keda.yaml](https://github.com/oam-dev/catalog/blob/master/registry/keda-scaler.yaml).
Several highlights are list below.

### 1. Describe The Trait

```yaml
...
name: keda-scaler
annotations:
  definition.oam.dev/description: "keda supports multiple event to elastically scale applications, this scaler only applies to deployment as example"
...
```

We use label `definition.oam.dev/description` to add one line description for this trait.
It will be shown in helper commands such as `$ vela traits`.

### 2. Register API Resource

```yaml
...
spec:
  definitionRef:
    name: scaledobjects.keda.sh
...
```

This is how you claim and register KEDA `ScaledObject`'s API resource (`scaledobjects.keda.sh`) as a trait definition.

### 3. Define `appliesToWorkloads`

A trait can be attached to specified workload types or all (i.e. `"*"` means your trait can work with any workload type).

For the case of KEAD, we will only allow user to attach it to Kubernetes workload type. So we claim it as below:

```yaml
...
spec:
  ...
  appliesToWorkloads:
    - "deployments.apps" # claim KEDA based autoscaling trait can only attach to Kubernetes Deployment workload type.
...
``` 

### 4. Define Schematic

In this step, we will define the schematic of KEDA based autoscaling trait, i.e. we will create abstraction for KEDA `ScaledObject` with simplified primitives, so end users of this platform don't really need to know what is KEDA at all. 


```yaml
...
schematic:
  cue:
    template: |-
      outputs: kedaScaler: {
      	apiVersion: "keda.sh/v1alpha1"
      	kind:       "ScaledObject"
      	metadata: {
      		name: context.name
      	}
      	spec: {
      		scaleTargetRef: {
      			name: context.name
      		}
      		triggers: [{
      			type: parameter.triggerType
      			metadata: {
      				type:  "Utilization"
      				value: parameter.value
      			}
      		}]
      	}
      }
      parameter: {
      	// +usage=Types of triggering application elastic scaling, Optional: cpu, memory
      	triggerType: string
      	// +usage=Value to trigger scaling actions, represented as a percentage of the requested value of the resource for the pods. like: "60"(60%)
      	value: string
      }
 ```

This is a CUE based template which only exposes `type` and `value` as trait properties for user to set.

> Please check the [Defining Trait with CUE](../cue/trait) section for more details regarding to CUE templating.

## Step 2: Register New Trait to KubeVela

As long as the definition file is ready, you just need to apply it to Kubernetes.

```bash
$ kubectl apply -f https://raw.githubusercontent.com/oam-dev/catalog/master/registry/keda-scaler.yaml
```

And the new trait will immediately become available for end users to use in `Application` resource.

