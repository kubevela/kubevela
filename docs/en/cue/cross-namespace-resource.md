---
title:  Define resources located in defferent namespace with application
---

In this section, we will introduce how to use cue template create resources (workload/trait) in different namespace with the application.

By default, the `metadata.namespace` of K8s resource in CuE template is automatically filled with the same namespace of the application.

If you want to create K8s resources running in a specific namespace witch is different with the application, you can set the `metadata.namespace` field.
KubeVela will create the resources in the specified namespace, and create a resourceTracker object as owener of those resources.


## Usage

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: worker
spec:
  definitionRef:
    name: deployments.apps
  schematic:
    cue:
      template: |
        parameter: {
        	name:  string
        	image: string
        	namespace: string  # make this parameter `namespace` as keyword which represents the resource maybe located in defferent namespace with application
        }
        output: {
        	apiVersion: "apps/v1"
        	kind:       "Deployment"
                metadata: {
                   namespace: my-namespace
                }
        	spec: {
        		selector: matchLabels: {
        			"app.oam.dev/component": parameter.name
        		}
        		template: {
        			metadata: labels: {
        				"app.oam.dev/component": parameter.name
        			}
        			spec: {
        				containers: [{
        					name:  parameter.name
        					image: parameter.image
        				}]
        			}}}
        }
```

