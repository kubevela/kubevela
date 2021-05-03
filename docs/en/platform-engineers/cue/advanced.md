---
title:  Advanced Features
---

As a Data Configuration Language, CUE allows you to do some advanced templating magic in definition objects.

## Render Multiple Resources With a Loop

You can define the for-loop inside the `outputs`.

> Note that in this case the type of `parameter` field used in the for-loop must be a map. 

Below is an example that will render multiple Kubernetes Services in one trait:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  name: expose
spec:
  schematic:
    cue:
      template: |
        parameter: {
          http: [string]: int
        }

        outputs: {
          for k, v in parameter.http {
            "\(k)": {
              apiVersion: "v1"
              kind:       "Service"
              spec: {
                selector:
                  app: context.name
                ports: [{
                  port:       v
                  targetPort: v
                }]
              }
            }
          }
        }
```

The usage of this trait could be:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: testapp
spec:
  components:
    - name: express-server
      type: webservice
      properties:
        ...
      traits:
        - type: expose
          properties:
            http:
              myservice1: 8080
              myservice2: 8081
```

## Execute HTTP Request in Trait Definition

The trait definition can send a HTTP request and capture the response to help you rendering the resource with keyword `processing`.

You can define HTTP request `method`, `url`, `body`, `header` and `trailer` in the `processing.http` section, and the returned data will be stored in `processing.output`.

> Please ensure the target HTTP server returns a **JSON data**.  `output`.

Then you can reference the returned data from `processing.output` in `patch` or `output/outputs`.

Below is an example:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  name: auth-service
spec:
  schematic:
    cue:
      template: |
        parameter: {
          serviceURL: string
        }

        processing: {
          output: {
            token?: string
          }
          // The target server will return a JSON data with `token` as key.
          http: {
            method: *"GET" | string
            url:    parameter.serviceURL
            request: {
              body?: bytes
              header: {}
              trailer: {}
            }
          }
        }

        patch: {
          data: token: processing.output.token
        }
```

In above example, this trait definition will send request to get the `token` data, and then patch the data to given component instance.

## Data Passing

A trait definition can read the generated API resources (rendered from `output` and `outputs`) of given component definition.

>  KubeVela will ensure the component definitions are always rendered before traits definitions.

Specifically, the `context.output` contains the rendered workload API resource (whose GVK is indicated by `spec.workload`in component definition), and use `context.outputs.<xx>` to contain all the other rendered API resources.

Below is an example for data passing:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: worker
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
                  ports: [{containerPort: parameter.port}]
                  envFrom: [{
                    configMapRef: name: context.name + "game-config"
                  }]
                  if parameter["cmd"] != _|_ {
                    command: parameter.cmd
                  }
                }]
              }
            }
          }
        }

        outputs: gameconfig: {
          apiVersion: "v1"
          kind:       "ConfigMap"
          metadata: {
            name: context.name + "game-config"
          }
          data: {
            enemies: parameter.enemies
            lives:   parameter.lives
          }
        }

        parameter: {
          // +usage=Which image would you like to use for your service
          // +short=i
          image: string
          // +usage=Commands to run in the container
          cmd?: [...string]
          lives:   string
          enemies: string
          port:    int
        }


---
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  name: ingress
spec:
  schematic:
    cue:
      template: |
        parameter: {
          domain:     string
          path:       string
          exposePort: int
        }
        // trait template can have multiple outputs in one trait
        outputs: service: {
          apiVersion: "v1"
          kind:       "Service"
          spec: {
            selector:
              app: context.name
            ports: [{
              port:       parameter.exposePort
              targetPort: context.output.spec.template.spec.containers[0].ports[0].containerPort
            }]
          }
        }
        outputs: ingress: {
          apiVersion: "networking.k8s.io/v1beta1"
          kind:       "Ingress"
          metadata:
              name: context.name
          labels: config: context.outputs.gameconfig.data.enemies
          spec: {
            rules: [{
              host: parameter.domain
              http: {
                paths: [{
                  path: parameter.path
                  backend: {
                    serviceName: context.name
                    servicePort: parameter.exposePort
                  }
                }]
              }
            }]
          }
        }
```

In detail, during rendering `worker` `ComponentDefinition`:
1. the rendered Kubernetes Deployment resource will be stored in the `context.output`,
2. all other rendered resources will be stored in `context.outputs.<xx>`, with `<xx>` is the unique name in every `template.outputs`.

Thus, in `TraitDefinition`, it can read the rendered API resources (e.g. `context.outputs.gameconfig.data.enemies`) from the `context`.
