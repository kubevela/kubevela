# Concepts and Glossaries

In the previous [Quick Start](en/quick-start.md) guide, we showed the end user experience KubeVela based platforms can provide.

However, the KubeVela project itself, is for *platform builders*. More accurately, KubeVela is as a framework to help platform team create such easy-to-use yet highly extensible application management experience to their end users. 

In following documentation, we will explain more about the idea of KubeVela and clarify some technical terms that are widely used in the project.

## Separate of Concerns

First of all, KubeVela introduces a workflow with separate of concerns as below:
- **Platform Team**: defining reusable templates for such as deployment environments and capabilities, and registering those templates into the cluster.
- **End Users**: choose a deployment environment, model the app with available capability templates, and deploy the app to target environment.

![alt](../resources/how-it-works.png)

Let's start from the *end users*!

## Appfile

For end users, we expect them to use simple and client-side tools to define the application with pre-defined templates. For example, GUI console, DSL, or any other developer friendly tool that can generate Kubernetes resources.

The `Appfile` we showed in the Quick Start guide is a demo purpose tool for this. A sample `Appfile` is as below:

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

    route: # trait
      domain: example.com
      rules:
        - path: /testapp
          rewriteTarget: /

  backend: # 2nd service
    type: task # workload type
    image: perl 
    cmd: ["perl",  "-Mbignum=bpi", "-wle", "print bpi(2000)"]
```

Under the hood, `Appfile` will generate an `Application` custom resource which is the main abstraction KubeVela exposed to end users.

> We generally consider *Appfile* out of the core scope of KubeVela and have no intention to promote it alone. But we *highly recommend* platform builders to provide such tool to your end users. For full schema of *Appfile*, please check its [reference documentation](developers/references/devex/appfile.md).

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

The design of `Application` is based on Open Application Model (OAM), we will explain it detail in below.

> Note that *Appfile* as a developer tool is designed to be a "superset" of *Application*. For example, the `build` section is *Appfile* specific.

### Workload Types

For each of the components in `Application`, its `.type` field represents the runtime characteristic of its workload (i.e. workload type) and `.settings` claims the configurations to initialize its workload instance. Some typical workload types are *Long Running Web Service* or *One-time Off Task*.

### Traits

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

Now that you have grasped the core ideas of KubeVela, let's learn more details about its [Application Definition and Encapsulation](platform-engineers/overview.md) feature.
