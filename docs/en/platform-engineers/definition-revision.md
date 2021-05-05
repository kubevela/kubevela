---
title:  Specify Definition Revision
---

Every time you update ComponentDefinition/TraitDefinition, a corresponding DefinitionRevision will be generated.
And the DefinitionRevision can be regarded as a snapshot of ComponentDefinition/TraitDefinition.

## Update Definition

Suppose we need a `worker` to run a background program.

```yaml
# worker.yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: worker
  namespace: default
  annotations:
    definition.oam.dev/description: "Describes long-running, scalable, containerized services that running at backend. They do NOT have network endpoint to receive external network traffic."
spec:
  workload:
    definition:
      apiVersion: batch/v1
      kind: Job
  schematic:
    cue:
      template: |
        output: {
          apiVersion: "batch/v1"
            kind:       "Job"
            spec: {
              parallelism: 1
                completions: 1
                template: spec: {
                  restartPolicy	: "Never"
                    containers: [{
                      name:  context.name
                        image: parameter.image
                        if parameter["cmd"] != _|_ {
                        command: parameter.cmd
                      }
                    }]
                }
            }
        }
        parameter: {
          image: string
          cmd?: [...string]
        }
```

```shell
kubectl apply -f worker.yaml
```

We can see there is a DefinitionRevision used to record the snapshot information of the current Definition.

```shell
$ kubectl get componentdefinition                            
NAME     WORKLOAD-KIND   DESCRIPTION
worker   Job             Describes long-running, scalable, containerized services that running at backend. They do NOT have network endpoint to receive external network traffic.

$ kubectl get definitionrevision               
NAME        REVISION   HASH               TYPE
worker-v1   1          76486234845427dc   Component
```

After a period of time, we want to use `Deployment` as the workload of the `worker`. Then we update the ComponentDefinition `worker`.

```yaml
# updated-worker.yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: worker
  namespace: default
  annotations:
    definition.oam.dev/description: "Describes long-running, scalable, containerized services that running at backend. They do NOT have network endpoint to receive external network traffic."
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
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
                        }]
                    }
                }
            }
        }
        parameter: {
            image: string
            cmd?: [...string]
        }
```

```shell
kubectl apply -f updated-worker.yaml
```

After updating the `worker`, A new DefinitionRevision is generated to record the snapshot information.
And We have generated corresponding configmaps for different versions of Definition to store parameter information.
The ConfigMap `schema-worker` will always be aligned with the parameter information of the latest Definition.

```shell
$ kubectl get definitionrevision
NAME        REVISION   HASH               TYPE
worker-v1   1          76486234845427dc   Component
worker-v2   2          cb22fdc3b037702e   Component

$ kubectl get configmap          
NAME               DATA   AGE
schema-worker      1      6m32s
schema-worker-v1   1      6m31s
schema-worker-v2   1      35s
```

## Specify Definition Version in Application

We can specify the Component to use a specific version of the Definition in the Application,
If no special declaration is made, the app will use the latest Definition to render the Component.

```yaml
# app.yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: worker-app
spec:
  components:
    - name: backend
      type: worker
      properties:
        image: busybox
        cmd:
          - sleep
          - '1000'
```

```shell
kubectl apply -f app.yaml
```

After apply the `app.yaml`, we can see that the resources `Deployment` is generated.

```shell
$ kubectl get deployment
NAME      READY   UP-TO-DATE   AVAILABLE   AGE
backend   1/1     1            1           8s
```

Sometimes the latest Definition may be unstable, so we want to use the v1 version of the worker to render the Component.
we can specify the version of Definition in format `definitionName@version`.

```yaml
# app.yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: worker-app
spec:
  components:
    - name: backend
      type: worker@v1
      properties:
        image: busybox
        cmd:
          - sleep
          - '1000'
```

```shell
kubectl apply -f app.yaml
```

After updating the Application `worker-app`.
We can see that vela uses the v1 version of the worker to render the Component and generates `Job` resources.

```shell
$ kubectl get jobs.batch
NAME      COMPLETIONS   DURATION   AGE
backend   0/1           3m36s      3m36s
```