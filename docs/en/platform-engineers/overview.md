---
title: Resource Model
---

This documentation will explain the core resource model of KubeVela which is fully powered by Open Application Model (OAM).

## Application

KubeVela introduces an `Application` CRD as its main API that captures a full application deployment. Every application is composed by multiple components with attachable operational behaviors (traits). For example:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: application-sample
spec:
  components:
    - name: foo
      type: webservice
      properties:
        image: crccheck/hello-world
        port: 8000
      traits:
        - type: ingress
          properties:
            domain: testsvc.example.com
            http:
              "/": 8000
        - type: sidecar
          properties:
            name: "logging"
            image: "fluentd"
    - name: bar
      type: aliyun-oss # cloud service
      bucket: "my-bucket"
```

Though the application object doesn't have fixed schema, it is a composition object assembled by several *programmable building blocks* as shown below.

## Component

The component model in KubeVela is designed to allow *component providers* to encapsulate deployable/provisionable entities by leveraging widely adopted tools such as CUE, Helm etc, and give a easier path to developers to deploy complicated microservices with ease.

Templates based encapsulation is probably the mostly widely used approach to enable efficient application deployment and exposes easier interfaces to end users. For example, many tools today encapsulate Kubernetes *Deployment* and *Service* into a *Web Service* module, and then instantiate this module by simply providing parameters such as *image=foo* and *ports=80*. This pattern can be found in cdk8s (e.g. [`web-service.ts` ](https://github.com/awslabs/cdk8s/blob/master/examples/typescript/web-service/web-service.ts)), CUE (e.g. [`kube.cue`](https://github.com/cuelang/cue/blob/b8b489251a3f9ea318830788794c1b4a753031c0/doc/tutorial/kubernetes/quick/services/kube.cue#L70)), and many widely used Helm charts (e.g. [Web Service](https://docs.bitnami.com/tutorials/create-your-first-helm-chart/)).

> Hence, a components provider could be anyone who packages software components in form of Helm chart of CUE modules. Think about 3rd-party software distributor, DevOps team, or even your CI pipeline.

In above example, it describes an application composed with Kubernetes stateless workload (component `foo`) and a Alibaba Cloud OSS bucket (component `bar`) alongside.

### How it Works?

In above example, `type: worker` means the specification of this component (claimed in following `properties` section) will be enforced by a `ComponentDefinition` object named `worker` as below:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: worker
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


Hence, the `properties` section of `backend` only exposes two parameters to fill: `image` and `cmd`, this is enforced by the `parameter` list of the `.spec.template` field of the definition.


## Traits

Traits are operational features that can be attached to component per needs. Traits are normally considered as platform features and maintained by platform team. In the above example, `type: autoscaler` in `frontend` means the specification (i.e. `properties` section)
of this trait will be enforced by a `TraitDefinition` object named `autoscaler` as below:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "configure k8s HPA for Deployment"
  name: hpa
spec:
  appliesToWorkloads:
    - webservice
    - worker
  schematic:
    cue:
      template: |
        outputs: hpa: {
          apiVersion: "autoscaling/v2beta2"
          kind:       "HorizontalPodAutoscaler"
          metadata: name: context.name
          spec: {
            scaleTargetRef: {
              apiVersion: "apps/v1"
              kind:       "Deployment"
              name:       context.name
            }
            minReplicas: parameter.min
            maxReplicas: parameter.max
            metrics: [{
              type: "Resource"
              resource: {
                name: "cpu"
                target: {
                  type:               "Utilization"
                  averageUtilization: parameter.cpuUtil
                }
              }
            }]
          }
        }
        parameter: {
          min:     *1 | int
          max:     *10 | int
          cpuUtil: *50 | int
        }
```

The application also have a `sidecar` trait.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "add sidecar to the app"
  name: sidecar
spec:
  appliesToWorkloads:
    - webservice
    - worker
  schematic:
    cue:
      template: |-
        patch: {
           // +patchKey=name
           spec: template: spec: containers: [parameter]
        }
        parameter: {
           name: string
           image: string
           command?: [...string]
        }
```

Please note that the end users of KubeVela do NOT need to know about definition objects, they learn how to use a given capability with visualized forms (or the JSON schema of parameters if they prefer). Please check the [Generate Forms from Definitions](/docs/platform-engineers/openapi-v3-json-schema) section about how this is achieved.

## Standard Contract Behind The Abstractions

Once the application is deployed, KubeVela will index and manage the underlying instances with name, revisions, labels and selector etc in automatic approach. These metadata are shown as below.

| Label  | Description |
| :--: | :---------: | 
|`workload.oam.dev/type=<component definition name>` | The name of its corresponding `ComponentDefinition` |
|`trait.oam.dev/type=<trait definition name>` | The name of its corresponding `TraitDefinition` | 
|`app.oam.dev/name=<app name>` | The name of the application it belongs to |
|`app.oam.dev/component=<component name>` | The name of the component it belongs to |
|`trait.oam.dev/resource=<name of trait resource instance>` | The name of trait resource instance |
|`app.oam.dev/appRevision=<name of app revision>` | The name of the application revision it belongs to |


Consider these metadata as a standard contract for any "day 2" operation controller such as rollout controller to work on KubeVela deployed applications. This is the key to ensure the interoperability for KubeVela based platform as well.

## No Configuration Drift

Despite the efficiency and extensibility in abstracting application deployment, IaC (Infrastructure-as-Code) tools may lead to an issue called *Infrastructure/Configuration Drift*, i.e. the generated component instances are not in line with the expected configuration. This could be caused by incomplete coverage, less-than-perfect processes or emergency changes. This makes them can be barely used as a platform level building block.

Hence, KubeVela is designed to maintain all these programmable capabilities with [Kubernetes Control Loop](https://kubernetes.io/docs/concepts/architecture/controller/) and leverage Kubernetes control plane to eliminate the issue of configuration drifting, while still keeps the flexibly and velocity enabled by IaC.
