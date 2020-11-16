# Extending Workload Types in KubeVela

> WARNINIG: you are now reading a platform builder/administrator oriented documentation.

In the following tutorial, you will learn how to add OpenFaaS Function a new workload type and expose it to users via Appfile.

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

### 4. Define User Parameters

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

For a given capability, KubeVela leverages [CUElang](https://github.com/cuelang/cue/blob/master/doc/tutorial/kubernetes/README.md)  to define the parameters that the end users could configure in the Appfile. In nutshell, `parameter.*` expected to be filled by users, and `context.name` will be filled by KubeVela as the service name in Appfile. 

> In the upcoming release, we will publish a detailed guide about defining CUE templates in KubeVela. For now, the best samples to learn about this section is the [built-in templates](https://github.com/oam-dev/kubevela/tree/master/hack/vela-templates) of KubeVela.

Note that OpenFaaS also requires a namespace and secret configured before first-time usage:

<details>

```bash
# generate a random password
$ PASSWORD=$(head -c 12 /dev/urandom | shasum| cut -d' ' -f1)

$ kubectl -n openfaas create secret generic basic-auth \
    --from-literal=basic-auth-user=admin \
    --from-literal=basic-auth-password="$PASSWORD"

$ kubectl apply -f https://raw.githubusercontent.com/openfaas/faas-netes/master/namespaces.yml
```
</details>

## Step 2: Register New Workload Type to KubeVela

As long as the definition file is ready, you just need to apply it to Kubernetes.

```bash
$ kubectl apply -f https://raw.githubusercontent.com/oam-dev/catalog/master/registry/openfaas.yaml
```

And the new workload type will immediately become available for developers to use in KubeVela.
It may take some time to be available as the dependency(helm chart) need to install.

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

## (Optional) Step 3: Deploy OpenFaaS Function via Appfile

Write an Appfile:

```bash
$ cat << EOF > vela.yaml
name: testapp
services:
  nodeinfo:
    type: openfaas
    image: functions/nodeinfo
    handler: "node main.js"
EOF
```

Deploy it:

```bash
$ vela up
```

(Optional) Verify the function is deployed and running.

<details>

Then you could find functions have been created:

```
$ kubectl get functions
NAME      AGE
nodeinfo   33s
```

Port-forward the OpenFaaS Gateway:

```
kubectl port-forward -n openfaas svc/gateway 31112:8080
```

Now you can visit OpenFaas dashboard via http://127.0.0.1:31112 .

Here is the login credential. Username is `admin`, and the password is set in previous step via `PASSWORD` env.
```
username: admin
password: $(echo $PASSWORD)
```

Then you can see the dashboard as below. The `nodeinfo` function is shown as well:

![alt](../../resources/openfaas.jpg)

</details>
