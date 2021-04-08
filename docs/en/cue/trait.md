---
title:  How-to
---

In this section we will introduce how to define a trait.

## Simple Trait

A trait in KubeVela can be defined by simply reference a existing Kubernetes API resource.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  name: ingress
spec:
  definitionRef:
    name: ingresses.networking.k8s.io
```
Let's attach this trait to a component instance in `Application`:

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
        cmd:
          - node
          - server.js
        image: oamdev/testapp:v1
        port: 8080
      traits:
        - type: ingress
          properties:
            rules:
            - http:
                paths:
                - path: /testpath
                  pathType: Prefix
                  backend:
                    service:
                      name: test
                      port:
                        number: 80
```

Note that in this case, all fields in the referenced resource's `spec` will be exposed to end user and no metadata (e.g. `annotations` etc) are allowed to be set trait properties. Hence this approach is normally used when you want to bring your own CRD and controller as a trait, and it dose not rely on `annotations` etc as tuning knobs.

## Using CUE as Trait Schematic

The recommended approach is defining a CUE based schematic for trait as well. In this case, it comes with abstraction and you have full flexibility to templating any resources and fields as you want. Note that KubeVela requires all traits MUST be defined in `outputs` section (not `output`) in CUE template with format as below:

```cue
outputs: <unique-name>: 
  <full template data>
```

Below is an example for `ingress` trait.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  name: ingress
spec:
  podDisruptive: false
  schematic:
    cue:
      template: |
        parameter: {
        	domain: string
        	http: [string]: int
        }

        // trait template can have multiple outputs in one trait
        outputs: service: {
        	apiVersion: "v1"
        	kind:       "Service"
        	spec: {
        		selector:
        			app: context.name
        		ports: [
        			for k, v in parameter.http {
        				port:       v
        				targetPort: v
        			},
        		]
        	}
        }

        outputs: ingress: {
        	apiVersion: "networking.k8s.io/v1beta1"
        	kind:       "Ingress"
        	metadata:
        		name: context.name
        	spec: {
        		rules: [{
        			host: parameter.domain
        			http: {
        				paths: [
        					for k, v in parameter.http {
        						path: k
        						backend: {
        							serviceName: context.name
        							servicePort: v
        						}
        					},
        				]
        			}
        		}]
        	}
        }
```

Let's attach this trait to a component instance in `Application`:

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
        cmd:
          - node
          - server.js
        image: oamdev/testapp:v1
        port: 8080
      traits:
        - type: ingress
          properties:
            domain: test.my.domain
            http:
              "/api": 8080
```

CUE based trait definitions can also enable many other advanced scenarios such as patching and data passing. They will be explained in detail in the following documentations.