# Extending Capabilities in KubeVela

## How Capabilities Work

A Capability is a functionality provided by the infrastructure that users can configure to run and operate applications.
Vela has [an extensible capability system](../design.md#2-capability-oriented-architecture) that allows platform builders to bring bespoke infrastructure capabilityes into Vela by writing YAML definitions and CUE templates.

In the following tutorial, you will learn how to add a new capability and expose it to users via CLI/Appfile.
The new capability is a type of trait but the same process applies to workload as well.

## Add A New Capability

Prerequisites:

- [helm v3](https://helm.sh/docs/intro/install/)
- [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/)

### Step 1: Install KubeWatch

```console
$ helm repo add vela-demo https://wonderflow.info/kubewatch/archives/
$ helm install kubewatch vela-demo/kubewatch --version 0.1.0
```

### Step 2: Add Trait Definition with CUE template

```console
$ cat << EOF | kubectl apply -f -
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  name: kubewatch
  annotations:
    definition.oam.dev/apiVersion: labs.bitnami.com/v1alpha1
    definition.oam.dev/kind: KubeWatch
    definition.oam.dev/description: "Add a watch for resource"
spec:
  appliesToWorkloads:
    - "*"
  workloadRefPath: spec.workloadRef
  definitionRef:
    name: kubewatches.labs.bitnami.com
  extension:
    template: |
      output: {
        apiVersion: "labs.bitnami.com/v1alpha1"
        kind:       "KubeWatch"
        spec: handler: webhook: url: parameter.webhook
      }
      parameter: {
        webhook: string
      }
EOF
```

That's it! Once you have applied the definition file the feature will be automatically registered in Vela Server and exposed to users.

### Step 3: Verify New Trait

Verify new trait:

```console
$ vela traits
Synchronizing capabilities from cluster⌛ ...
Sync capabilities successfully ✅ Add(1) Update(0) Delete(0)
TYPE      	CATEGORY	DESCRIPTION
+kubewatch	trait   	Add a watch for resource

Listing trait capabilities ...

NAME     	DESCRIPTION                       	APPLIES TO
kubewatch	Add a watch for resource
...
```

### Step 4: Adding Kubewatch Trait to The App

Write an Appfile:

```console
$ cat << EOF > vela.yaml
name: testapp
services:
  testsvc:
    type: webservice
    image: crccheck/hello-world
    port: 8000
    route:
      domain: testsvc.example.com
EOF
```

Deploy it:

```console
$ vela up
...
✅ App has been deployed 🚀🚀🚀
    Port forward: vela port-forward testapp
             SSH: vela exec testapp
         Logging: vela logs testapp
      App status: vela status testapp
  Service status: vela status testapp --svc testsvc
```

You can use either of the following options to attach the newly added kubewatch trait to the App:

#### Option 1: Testing in CLI

Add kubewatch trait to the application:

```console
$ vela kubewatch testapp --svc testsvc --webhook https://hooks.slack.com/<your-token>
Adding kubewatch for app testsvc
⠋ Checking Status ...
✅ Application Deployed Successfully!
  - Name: testsvc
    Type: webservice
    HEALTHY Ready: 1/1
    Traits:
      - ✅ kubewatch: webhook=https://hooks.slack.com/...
  ...
```

Check your Slack channel to verify the nofitications:

![Image of Kubewatch](../../resources/kubewatch-notif.jpg)

#### Option 2: Testing in Appfile

Instead of using CLI, you can add `kubewatch` config to Appfile:

```yaml
$ cat << EOF >> vela.yaml
    kubewatch:
      webhook: https://hooks.slack.com/<your-token>
EOF
```

Deploy it:

```
$ vela up
```
