---
title:  Extend CRD Operator as Component Type
---

Let's use [OpenKruise](https://github.com/openkruise/kruise) as example of extend CRD as KubeVela Component.
**The mechanism works for all CRD Operators**.

### Step 1: Install the CRD controller

You need to [install the CRD controller](https://github.com/openkruise/kruise#quick-start) into your K8s system.

### Step 2: Create Component Definition

To register Cloneset(one of the OpenKruise workloads) as a new workload type in KubeVela, the only thing needed is to create an `ComponentDefinition` object for it.
A full example can be found in this [cloneset.yaml](https://github.com/oam-dev/catalog/blob/master/registry/cloneset.yaml).
Several highlights are list below.

#### 1. Describe The Workload Type

```yaml
...
  annotations:
    definition.oam.dev/description: "OpenKruise cloneset"
...
```

A one line description of this component type. It will be shown in helper commands such as `$ vela components`.

#### 2. Register it's underlying CRD

```yaml
...
workload:
  definition:
    apiVersion: apps.kruise.io/v1alpha1
    kind: CloneSet
...
```

This is how you register OpenKruise Cloneset's API resource (`fapps.kruise.io/v1alpha1.CloneSet`) as the workload type.
KubeVela uses Kubernetes API resource discovery mechanism to manage all registered capabilities.

#### 4. Define Template

```yaml
...
schematic:
  cue:
    template: |
      output: {
          apiVersion: "apps.kruise.io/v1alpha1"
          kind:       "CloneSet"
          metadata: labels: {
            "app.oam.dev/component": context.name
          }
          spec: {
              replicas: parameter.replicas
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
                    }]
                  }
              }
          }
      }
      parameter: {
          // +usage=Which image would you like to use for your service
          // +short=i
          image: string

          // +usage=Number of pods in the cloneset
          replicas: *5 | int
      }
 ```

### Step 3: Register New Component Type to KubeVela

As long as the definition file is ready, you just need to apply it to Kubernetes.

```bash
$ kubectl apply -f https://raw.githubusercontent.com/oam-dev/catalog/master/registry/cloneset.yaml
```

And the new component type will immediately become available for developers to use in KubeVela.
