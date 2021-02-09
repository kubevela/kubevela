# Concepts and Glossaries

With great end user experience, KubeVela itself is built for *platform builders*. More accurately, it's a framework to help platform team create easy-to-use yet highly extensible application management experience showed in the [Quick Start](en/quick-start.md) guide.

This document explains some technical terms that are widely used in KubeVela and clarify them in the context of platform builders.

## Appfile

In the [Quick Start](en/quick-start.md) guide, we showed how to use a docker-compose style YAML file to define and deploy the application. This YAML file is a client-side tool named `Appfile` to render custom resources of KubeVela. The alternative of `Appfile` could be GUI console, DSL, or any other developer friendly tool that can generate Kubernetes objects. We **highly recommend** platform builders to provide such tools to your end users instead of exposing Kubernetes and explain how KubeVela made this effort super easy step by step.

A simple `Appfile` sample is as below:

```yaml
name: testapp

services:
  frontend: # 1st service
    image: oamdev/testapp:v1

    build:
      docker:
        file: Dockerfile
        context: .

    cmd: ["node", "server.js"]
    port: 8080

    route: # a route trait
      domain: example.com
      rules:
        - path: /testapp
          rewriteTarget: /

  backend: # 2nd service
    type: task
    image: perl 
    cmd: ["perl",  "-Mbignum=bpi", "-wle", "print bpi(2000)"]
```

Note that `Appfile` as a developer tool is designed as a "superset" of `Application`, for example, developers can define a `build` section in `Appfile` which is not part of `Application` CRD.

> For full schema of `Appfile`, please check its [reference documentation](developers/references/devex/appfile.md).

## Separate of Concerns

KubeVela follows a workflow with separate of concerns as below:
- Platform team: defining reusable templates for such as deployment environments and capabilities, and registering those templates into the cluster.
- End users: choose a deployment environment, model the app with available capability templates, and deploy the app to target environment.

![alt](../resources/how-it-works.png)

## Application
The *Application* is the core API of KubeVela. It is an abstraction that allows developers to work with a single artifact to capture the complete application definition.

This is important to simplify administrative tasks and can serve as an anchor to avoid configuration drifts during operation. Also, as an abstraction object, `Application` provided a much simpler path for on-boarding Kubernetes capabilities without relying on low level details. For example, a developer will be able to model a "web service" without defining detailed Kubernetes Deployment + Service combo each time, or claim the auto-scaling requirements without referring to the underlying KEDA ScaleObject.

An example of `website` application with two components (i.e. `frontend` and `backend`) could be modeled as below:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: Application
metadata:
  name: website
spec:
  components:
    - name: backend
      type: worker
      settings:
        image: busybox
        cmd:
          - sleep
          - '1000'
    - name: frontend
      type: webservice
      settings:
        image: nginx
      traits:
        - name: autoscaler
          properties:
            min: 1
            max: 10
        - name: sidecar
          properties:
            name: "sidecar-test"
            image: "fluentd"
```

The design of `Application` adopts Open Application Model (OAM).

### Workload Type

For each of the components, its `.type` field represents the runtime characteristic of its workload (i.e. workload type) and `.settings` claims the configurations to initialize its workload instance. Some typical workload types are *Long Running Web Service* or *One-time Off Task*.

### Trait

Optionally, each component has a `.traits` section that augments its workload instance with operational behaviors such as load balancing policy, network ingress routing, auto-scaling policies, or upgrade strategies, etc. Its `.name` field references the specific trait definition, and `.properties` sets detailed configuration values of the given trait.

We also reference workload type and trait as "capabilities" in KubeVela.

## Definitions

Both the schemas of workload settings and trait properties in `Application` are enforced by capability templates that are pre-defined separately by platform team in a set of definition objects. The platform team is responsible for registering and managing definitions in the cluster following [workload definition](https://github.com/oam-dev/spec/blob/master/4.workload_definitions.md) and [trait definition](https://github.com/oam-dev/spec/blob/master/6.traits.md) specifications in Open Application Model (OAM). 

For example, a `worker` workload type could be defined by a `WorkloadDefinition` as below:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: worker
  annotations:
    definition.oam.dev/description: "Long-running scalable backend worker without network endpoint"
spec:
  definitionRef:
    name: deployments.apps
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
    	// +usage=Which image would you like to use for your service
    	// +short=i
    	image: string

    	cmd?: [...string]
    }
```

Once this definition is applied to the cluster, the end users will be able to claim a component with workload type of `worker`, and fill in the properties as below:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: Application
metadata:
  name: website
spec:
  components:
    - name: backend
      type: worker
      settings:
        image: busybox
        cmd:
          - sleep
          - '1000'
```

Currently, KubeVela supports [CUE](https://github.com/cuelang/cue) as the templating language in definitions. In the upcoming releases, it will also support referencing Helm chart as workload/trait definition. In this case, the chart's `values.yaml` will be exposed as application properties directly.

## Environment
Before releasing an application to production, it's important to test the code in testing/staging workspaces. In KubeVela, we describe these workspaces as "deployment environments" or "environments" for short. Each environment has its own configuration (e.g., domain, Kubernetes cluster and namespace, configuration data, access control policy etc.) to allow user to create different deployment environments such as "test" and "production".

Currently, a KubeVela `environment` only maps to a Kubernetes namespace, while the cluster level environment is work in progress.

### Summary

The main concepts of KubeVela could be shown as below:

![alt](../resources/concepts.png)

## Architecture

The overall architecture of KubeVela is shown as below:

![alt](../../resources/kubevela-runtime.png)

The encapsulation engine in KubeVela is responsible for application abstraction and encapsulation (i.e. the controller for `Application` and `Definition`).

The deployment engine (*currently WIP*) is responsible for progressive rollout of the application (i.e. the controller for `AppDeployment`).


## What's Next

Now that you have grasped the core ideas of KubeVela. Let's learn more about KubeVela start from its [Application Definition and Encapsulation](platform-engineers/overview.md).
