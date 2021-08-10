---
title: Writing X-Definitions with Vela CLI
---

## Introduction

`vela def` is a group of commands designed for helping users managing definitions in KubeVela.

## Background

In KubeVela, the main capability of definition is defined by the cue-format template. However, as a JSON-based language, cue-format template is not capatible with the original Kubernetes YAML format, which means we need to transform cue-format template into raw string while embedding it into the Kubernetes YAML objects. This made it relatively difficult to view or edit KubeVela definitions through native **kubectl** tool. For example, when we access a trait definition that attach labels to components, we can run `kubectl get trait labels -n vela-system -o yaml`, the result is as below

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: Add labels for your Workload.
    meta.helm.sh/release-name: kubevela
    meta.helm.sh/release-namespace: vela-system
  creationTimestamp: "2021-08-05T07:06:58Z"
  generation: 1
  labels:
    app.kubernetes.io/managed-by: Helm
  name: labels
  namespace: vela-system
  resourceVersion: "8423"
  uid: 51a7f8b1-f14d-4776-b538-02ac54a55661
spec:
  appliesToWorkloads:
  - deployments.apps
  podDisruptive: true
  schematic:
    cue:
      template: "patch: spec: template: metadata: labels: {\n\tfor k, v in parameter
        {\n\t\t\"\\(k)\": v\n\t}\n}\nparameter: [string]: string\n"
status:
  configMapRef: schema-labels
  latestRevision:
    name: labels-v1
    revision: 1
    revisionHash: fe7fa9da440dc9d3
```

We can see that the core ability of the *labels* definition locates at `spec.schematic.cue.template` and is escaped into raw string. It is hard to identify and manipulate.

On the other hand, although cuelang has very strong expression capabilities, it is still relatively new to many Kubernetes developers who might takes some extra time to learn when leveraging the power of cuelang.

Therefore, KubeVela team introduce a series of functions in the `vela` CLI tool, aiming to help developers design all kinds of definitions conveniently.

## Design

As mentioned above, the CUE-YAML-mixed-style KubeVela definition, is represented by a single cue-format definition file in v1.1, which expresses the content and description of defintiion more clearly and briefly. The `labels` definition above can be represented by the following file

```json
// labels.cue
labels: {
        annotations: {}
        attributes: {
                appliesToWorkloads: ["deployments.apps"]
                podDisruptive: true
        }
        description: "Add labels for your Workload."
        labels: {}
        type: "trait"
}
template: {
        patch: spec: template: metadata: labels: {
                for k, v in parameter {
                        "\(k)": v
                }
        }
        parameter: [string]: string
}
```

The first part `labels: {...}` expresses the basic information of the definition, including its type, description, labels, annotations and attributes. The second part `template: {...}` describes the capability of the definition. With the `vela def` command group，KubeVela users can directly interact with the CUE-format file instead of facing the complex YAML file.

## Details

### init

`vela def init` is a command that helps users bootstrap new definitions. To create an empty trait definition, run `vela def init my-trait -t trait --desc "My trait description."`

```json
"my-trait": {
        annotations: {}
        attributes: {
                appliesToWorkloads: []
                conflictsWith: []
                definitionRef:   ""
                podDisruptive:   false
                workloadRefPath: ""
        }
        description: "My trait description."
        labels: {}
        type: "trait"
}
template: {
        patch: {}
        parameter: {}
}
```

Or you can use `vela def init my-comp --interactive` to initiate definitions interactively.

```bash
$ vela def init my-comp --interactive
Please choose one definition type from the following values: component, trait, policy, workload, scope, workflow-step
> Definition type: component
> Definition description: My component definition.
Please enter the location the template YAML file to build definition. Leave it empty to generate default template.
> Definition template filename: 
Please enter the output location of the generated definition. Leave it empty to print definition to stdout.
> Definition output filename: my-component.cue
Definition written to my-component.cue
```

In addition, users can create definitions from existing YAML files. For example, if a user want to create a ComponentDefinition which is designed to generate a deployment, and this deployment has already been created elsewhere, he/she can use the `--template-yaml` flag to complete the transformation. The YAML file is as below

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hello-world
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: hello-world
  template:
    metadata:
      labels:
        app.kubernetes.io/name: hello-world
    spec:
      containers:
      - name: hello-world
        image: somefive/hello-world
        ports: 
        - name: http
          containerPort: 80
          protocol: TCP
---
apiVersion: v1
kind: Service
metadata:
  name: hello-world-service
spec:
  selector:
    app: hello-world
  ports:
  - name: http
    protocol: TCP
    port: 80
    targetPort: 8080
  type: LoadBalancer
```

Running `vela def init my-comp -t component --desc "My component." --template-yaml ./my-deployment.yaml` to get the CUE-format ComponentDefinition

```json
"my-comp": {
        annotations: {}
        attributes: workload: definition: {
                apiVersion: "<change me> apps/v1"
                kind:       "<change me> Deployment"
        }
        description: "My component."
        labels: {}
        type: "component"
}
template: {
        output: {
                metadata: name: "hello-world"
                spec: {
                        replicas: 1
                        selector: matchLabels: "app.kubernetes.io/name": "hello-world"
                        template: {
                                metadata: labels: "app.kubernetes.io/name": "hello-world"
                                spec: containers: [{
                                        name:  "hello-world"
                                        image: "somefive/hello-world"
                                        ports: [{
                                                name:          "http"
                                                containerPort: 80
                                                protocol:      "TCP"
                                        }]
                                }]
                        }
                }
                apiVersion: "apps/v1"
                kind:       "Deployment"
        }
        outputs: "hello-world-service": {
                metadata: name: "hello-world-service"
                spec: {
                        ports: [{
                                name:       "http"
                                protocol:   "TCP"
                                port:       80
                                targetPort: 8080
                        }]
                        selector: app: "hello-world"
                        type: "LoadBalancer"
                }
                apiVersion: "v1"
                kind:       "Service"
        }
        parameter: {}

}
```

Then the user can make further modifications based on the definition file above, like removing *\<change me\>* in **workload.definition**。

### vet

After initializing definition files, run `vela def vet my-comp.cue` to validate if there are any syntax error in the definition file. It can be used to detect some simple errors such as missing brackets.

```bash
$ vela def vet my-comp.cue
Validation succeed.
```

### render / apply

After confirming the definition file has correct syntax. users can run  `vela def apply my-comp.cue --namespace my-namespace` to apply this definition in the `my-namespace` namespace。If you want to check the transformed Kubernetes YAML file, `vela def apply my-comp.cue --dry-run` or `vela def render my-comp.cue -o my-comp.yaml` can achieve that.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  annotations:
    definition.oam.dev/description: My component.
  labels: {}
  name: my-comp
  namespace: vela-system
spec:
  schematic:
    cue:
      template: |
        output: {
                metadata: name: "hello-world"
                spec: {
                        replicas: 1
                        selector: matchLabels: "app.kubernetes.io/name": "hello-world"
                        template: {
                                metadata: labels: "app.kubernetes.io/name": "hello-world"
                                spec: containers: [{
                                        name:  "hello-world"
                                        image: "somefive/hello-world"
                                        ports: [{
                                                name:          "http"
                                                containerPort: 80
                                                protocol:      "TCP"
                                        }]
                                }]
                        }
                }
                apiVersion: "apps/v11"
                kind:       "Deployment"
        }
        outputs: "hello-world-service": {
                metadata: name: "hello-world-service"
                spec: {
                        ports: [{
                                name:       "http"
                                protocol:   "TCP"
                                port:       80
                                targetPort: 8080
                        }]
                        selector: app: "hello-world"
                        type: "LoadBalancer"
                }
                apiVersion: "v1"
                kind:       "Service"
        }
        parameter: {}
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
```

```bash
$ vela def apply my-comp.cue -n my-namespace
ComponentDefinition my-comp created in namespace my-namespace.
```

### get / list / edit / del

While you can use native kubectl tools to confirm the results of the apply command, as mentioned above, the YAML object mixed with raw CUE template string is complex. Using `vela def get` will automatically convert the YAML object into the CUE-format definition.

```bash
$ vela def get my-comp -t component
```

Or you can list all defintions installed through `vela def list`

```bash
$ vela def list -n my-namespace -t component
NAME                    TYPE                    NAMESPACE       DESCRIPTION  
my-comp                 ComponentDefinition     my-namespace    My component.
```

Similarly, using `vela def edit` to edit definitions in pure CUE-format. The transformation between CUE-format definition and YAML object is done by the command. Besides, you can specify the `EDITOR` environment variable to use your favourate editor.

```bash
$ EDITOR=vim vela def edit my-comp
```

Finally, `vela def del` can be utilized to delete existing definitions.

```bash
$ vela def del my-comp -n my-namespace  
Are you sure to delete the following definition in namespace my-namespace?
ComponentDefinition my-comp: My component.
[yes|no] > yes
ComponentDefinition my-comp in namespace my-namespace deleted.
```

