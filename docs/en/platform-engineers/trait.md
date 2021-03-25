# Trait Definition

In the following tutorial, you will learn about define your own trait to extend KubeVela.

Before continue, make sure you have learned the basic concept of [Definition Objects](definition-and-templates.md) in KubeVela.

The KubeVela trait system is very powerful. Generally, you could define a trait(e.g. "do some patch") with very low code,
just writing some CUE template is enough. Refer to ["Defining Traits in CUE"](https://kubevela.io/#/en/cue/trait) for
more details in this case.


## Extend CRD Operator as Trait

In the following tutorial, you will learn to extend traits into KubeVela with [KEDA](https://keda.sh/) as example.
KEDA is a very cool Event Driven Autoscaler.

### Step 1: Install the CRD controller

[Install the KEDA controller](https://keda.sh/docs/2.2/deploy/) into your K8s system.

### Step 2: Create Trait Definition

To register KEDA as a new trait in KubeVela, the only thing needed is to create an `TraitDefinition` object for it.

A full example can be found in this [keda.yaml](https://github.com/oam-dev/catalog/blob/master/registry/keda.yaml).
Several highlights are list below.

#### 1. Describe The Trait Usage

```yaml
...
  name: kubewatch
  annotations:
    definition.oam.dev/description: "Add a watch for resource"
...
```

We use label `definition.oam.dev/description` to add one line description for this trait.
It will be shown in helper commands such as `$ vela traits`.

#### 2. Register API Resource

```yaml
...
spec:
  definitionRef:
    name: kubewatches.labs.bitnami.com
...
```

This is how you register Kubewatch's API resource (`kubewatches.labs.bitnami.com`) as the Trait.


KubeVela uses Kubernetes API resource discovery mechanism to manage all registered capabilities.



#### 3. Define Workloads this trait can apply to

```yaml
...
spec:
  ...
  appliesToWorkloads:
    - "*"
...
```

A trait can work on specified workload or any kinds of workload, that deponds on what you describe here.
Use `"*"` to represent your trait can work on any workloads. 

You can also specify the trait can only work on K8s Deployment and Statefulset by describe like below:

```yaml
...
spec:
  ...
  appliesToWorkloads:
    - "deployments.apps"
    - "statefulsets.apps"
...
``` 

#### 4. Define the field if the trait can receive workload reference

```yaml
...
spec:
  workloadRefPath: spec.workloadRef
...
```

Once registered, the OAM framework can inject workload reference information automatically to trait CR object during creation or update.
The workload reference will include group, version, kind and name. Then, the trait can get the whole workload information
from this reference.

With the help of the OAM framework, end users will never bother writing the relationship info such like `targetReference`.
Platform builders only need to declare this info here once, then the OAM framework will help glue them together.

#### 5. Define Template

```yaml
...
    template: |
      outputs: kubewatch: {
        apiVersion: "labs.bitnami.com/v1alpha1"
        kind:       "KubeWatch"
        spec: handler: webhook: url: parameter.webhook
      }
      parameter: {
        webhook: string
      }
 ```

This is a CUE based template to define end user abstraction for this workload type. Please check the [templating documentation](../cue/trait.md) for more detail.

Note that in this example, we only need to give the webhook url as parameter for using KubeWatch.

### Step 2: Register New Trait to KubeVela

As long as the definition file is ready, you just need to apply it to Kubernetes.

```bash
$ kubectl apply -f https://raw.githubusercontent.com/oam-dev/catalog/master/registry/kubewatch.yaml
```

And the new trait will immediately become available for developers to use in KubeVela.

