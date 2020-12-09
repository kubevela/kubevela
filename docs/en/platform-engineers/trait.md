# Extending Traits in KubeVela

> WARNINIG: you are now reading a platform builder/administrator oriented documentation.

In the following tutorial, you will learn how to add a new trait and expose it to users via Appfile.
We will use [KubeWatch](https://github.com/wonderflow/kubewatch) as example, this is a forked version
that we make it work as CRD controller. User could use CRD(`kubewatches.labs.bitnami.com`) to describe K8s resources they
want to watch including any types of CRD.

## Step 1: Create Trait Definition

To register [KubeWatch](https://github.com/wonderflow/kubewatch) as a new trait in KubeVela,
the only thing needed is to create an OAM `TraitDefinition` object for it.
A full example can be found in this [kubewatch.yaml](https://github.com/oam-dev/catalog/blob/master/registry/kubewatch.yaml).
Several highlights are list below.

### 1. Describe The Trait Usage

```yaml
...
  name: kubewatch
  annotations:
    definition.oam.dev/description: "Add a watch for resource"
...
```

We use label `definition.oam.dev/description` to add one line description for this trait.
It will be shown in helper commands such as `$ vela traits`.

### 2. Register API Resource

```yaml
...
spec:
  definitionRef:
    name: kubewatches.labs.bitnami.com
...
```

This is how you register Kubewatch's API resource (`kubewatches.labs.bitnami.com`) as the Trait.


KubeVela uses Kubernetes API resource discovery mechanism to manage all registered capabilities.


### 3. Configure Installation Dependency

```yaml
...
  extension:
    install:
      helm:
        repo: my-repo
        name: kubewatch
        url: https://wonderflow.info/kubewatch/archives/
        version: 0.1.0
        ...
```

The `extension.install` field is used by KubeVela to automatically install the dependency (if any) when the new workload
type added to KubeVela. The dependency is described by a Helm chart custom resource.
We highly recommend you to configure this field since otherwise,
users will have to install dependencies like this kubewatch controller manually later to user your new trait.

### 4. Define Workloads this trait can apply to

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

### 5. Define the field if the trait can receive workload reference

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

### 6. Define User Parameters

```yaml
...
    template: |
      output: {
        apiVersion: "labs.bitnami.com/v1alpha1"
        kind:       "KubeWatch"
        spec: handler: webhook: url: parameter.webhook
      }
      parameter: {
        webhook: string
      }
 ```

For a given capability, KubeVela leverages [CUElang](https://github.com/cuelang/cue/blob/master/doc/tutorial/kubernetes/README.md)
to define the parameters that the end users could configure in the Appfile.
In nutshell, `parameter.*` expected to be filled by users. 

> In the upcoming release, we will publish a detailed guide about defining CUE templates in KubeVela.
> For now, the best samples to learn about this section is the [built-in templates](https://github.com/oam-dev/kubevela/tree/master/hack/vela-templates) of KubeVela.

Note that in this example, we only need to give the webhook url as parameter for using KubeWatch.

## Step 2: Register New Trait to KubeVela

As long as the definition file is ready, you just need to apply it to Kubernetes.

```bash
$ kubectl apply -f https://raw.githubusercontent.com/oam-dev/catalog/master/registry/kubewatch.yaml
```

And the new trait will immediately become available for developers to use in KubeVela.
It may take some time to be available as the dependency(helm chart) need to install.

```bash
$ vela traits
"my-repo" has been added to your repositories
Successfully installed chart (kubewatch) with release name (kubewatch)
Automatically discover capabilities successfully ✅ Add(1) Update(0) Delete(0)

TYPE      	CATEGORY	DESCRIPTION
+kubewatch	trait   	Add a watch for resource

NAME     	DESCRIPTION                                                      	APPLIES TO
autoscale	Automatically scale the app following certain triggers or metrics	webservice
         	                                                                 	worker
kubewatch	Add a watch for resource
metrics  	Configure metrics targets to be monitored for the app            	webservice
         	                                                                 	task
rollout  	Configure canary deployment strategy to release the app          	webservice
route    	Configure route policy to the app                                	webservice
scaler   	Manually scale the app                                           	webservice
        	                                                                 	worker
```

## Step 3: Using the new extended trait

Using the extended trait is just the same as the trait installed from Capability center, please [refer there to see how
to use](../developers/cap-center.md#Use-the-newly-installed-capability).
