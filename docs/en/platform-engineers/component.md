# Component Definition

In the following tutorial, you will learn about define your own Component Definition to extend KubeVela.

Before continue, make sure you have learned the basic concept of [Definition Objects](definition-and-templates.md) in KubeVela.

There're 

## Step 1: Create Workload Definition

To register OpenFaaS as a new workload type in KubeVela, the only thing needed is to create an OAM `WorkloadDefinition` object for it. A full example can be found in this [openfaas.yaml](https://github.com/oam-dev/catalog/blob/master/registry/openfaas.yaml). Several highlights are list below.

### 1. Describe The Workload Type

```yaml
...
  annotations:
    definition.oam.dev/description: "OpenFaaS function"
...
```

A one line description of this workload type. It will be shown in helper commands such as `$ vela workloads`.

### 2. Register API Resource

```yaml
...
spec:
  definitionRef:
    name: functions.openfaas.com
...
```

This is how you register OpenFaaS Function's API resource (`functions.openfaas.com`) as the workload type. KubeVela uses Kubernetes API resource discovery mechanism to manage all registered capabilities.


### 3. Configure Installation Dependency

```yaml
...
  extension:
    install:
      helm:
        repo: openfaas
        name: openfaas
        namespace: openfaas
        url: https://openfaas.github.io/faas-netes/
        version: 6.1.2
        ...
```

The `extension.install` field is used by KubeVela to automatically install the dependency (if any) when the new workload type is added to KubeVela. The dependency is described by a Helm chart custom resource. We highly recommend you to configure this field since otherwise, users will have to install dependencies like OpenFaaS operator manually later to user your new workload type.

### 4. Define Template

```yaml
...
    template: |
      output: {
        apiVersion: "openfaas.com/v1"
        kind:       "Function"
        spec: {
          handler: parameter.handler
          image: parameter.image
          name: context.name
        }
      }
      parameter: {
        image: string
        handler: string
      }
 ```

This is a CUE based template to define end user abstraction for this workload type. Please check the [templating documentation](../cue/workload-type.md) for more detail.

Note that OpenFaaS also requires a namespace and secret configured before first-time usage:

<details>

```bash
# create namespace
$ kubectl apply -f https://raw.githubusercontent.com/openfaas/faas-netes/master/namespaces.yml

# generate a random password
$ PASSWORD=$(head -c 12 /dev/urandom | shasum| cut -d' ' -f1)

$ kubectl -n openfaas create secret generic basic-auth \
    --from-literal=basic-auth-user=admin \
    --from-literal=basic-auth-password="$PASSWORD"
```
</details>

## Step 2: Register New Workload Type to KubeVela

As long as the definition file is ready, you just need to apply it to Kubernetes.

```bash
$ kubectl apply -f https://raw.githubusercontent.com/oam-dev/catalog/master/registry/openfaas.yaml
```

And the new workload type will immediately become available for developers to use in KubeVela.
It may take some time to be available as the dependency (helm chart) need to install.

## Step 3: Verify

```bash
$ vela workloads
Successfully installed chart (openfaas) with release name (openfaas)
"my-repo" has been added to your repositories
Automatically discover capabilities successfully ✅ Add(1) Update(0) Delete(0)

TYPE     	CATEGORY	DESCRIPTION
+openfaas	workload	OpenFaaS function workload

NAME      	DESCRIPTION
openfaas  	OpenFaaS function workload
task      	One-off task to run a piece of code or script to completion
webservice	Long-running scalable service with stable endpoint to receive external traffic
worker    	Long-running scalable backend worker without network endpoint
```
