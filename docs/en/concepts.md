# Concepts and Glossaries

This document explains some technical terms that are widely used in KubeVela. The goal is to clarify them for platform builders in the context of KubeVela.

## Separate of Concerns

KubeVela follows a workflow with separate of concerns as below:
- Platform team: defining reusable templates for such as deployment environments and capabilities, and registering those templates into the cluster.
- End users: choose a deployment environment, model the app with available capability templates, and deploy the app to target environment.

![alt](../resources/how-it-works.png)


## Application
An *application* in KubeVela is an abstraction that allows developers to work with a single artifact to capture the complete application definition.

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

### Workload Type

For each of the components, its `.type` field represents the runtime characteristic of its workload (i.e. workload type) and `.settings` claims the configurations to initialize its workload instance. Some typical workload types are *Long Running Web Service* or *One-time Off Task*.

### Trait

Optionally, each component has a `.traits` section that augments its workload instance with operational features such as load balancing policy, network ingress routing, auto-scaling policies, or upgrade strategies, etc. Its `.name` field references the specific trait definition, and `.properties` sets detailed configuration values of the given trait.

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

## Appfile

To help developers design and describe an application with ease, KubeVela also provided a client-side tool named `Appfile` to render the `Application` resource. A simple `Appfile` sample is as below:

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

Note that `Appfile` as a developer tool is designed as a "superset" of `Application`, for example, developers can define a `build` section in `Appfile` which is not part of `Application` CRD. For full schema of `Appfile`, please check its [ reference documentation](developers/references/devex/appfile.md) for more detail.


## Environment
Before releasing an application to production, it's important to test the code in testing/staging workspaces. In KubeVela, we describe these workspaces as "deployment environments" or "environments" for short. Each environment has its own configuration (e.g., domain, Kubernetes cluster and namespace, configuration data, access control policy etc.) to allow user to create different deployment environments such as "test" and "production".

Currently, a KubeVela `environment` only maps to a Kubernetes namespace, while the cluster level environment is on the way.

## Summary

The relationship of the main concepts in KubeVela could be shown as below:

![alt](../resources/concepts.png)


## What's Next

Now that you have grasped the core ideas of KubeVela. Here are some recommended next steps:

- Learn more about KubeVela through its [platform builder guide](platform-engineers/overview.md)
- Continue to try out [end user tutorials](developers/learn-appfile.md) to experience what KubeVela can be used to build
