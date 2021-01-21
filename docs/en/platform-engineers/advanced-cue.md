# Using CUE to Extend Trait in advanced way

> WARNINIG: you are now reading a platform builder/administrator oriented documentation.

In the following tutorial, you will learn how to add a trait in a more advanced way without writing any CRD controller.
In general, the advanced way can help you build abstraction by composition or decomposition.

## Trait generate multiple resources

With the help of CUE template, we can combine multiple K8s resources into one trait.
 
You can use the keyword `outputs` to create multiple K8s objects. The format MUST be `outputs:<unique-name>:<k8s-object>`.

Let's look at an example, assume you hope to make a combo for K8s service and ingress, naming it as `new-route`.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  name: new-route
spec:
  extension:
    template: |
      parameter: {
        domain: string
        http: [string]: int
      }

      // trait template can have multiple outputs in one trait
      outputs: service: {
        apiVersion: "v1"
        kind: "Service"
        spec: {
          selector:
            app: context.name
          ports: [
            for k, v in parameter.http {
              port: v
              targetPort: v
            }
          ]
        }
      }

      outputs: ingress: {
        apiVersion: "networking.k8s.io/v1beta1"
        kind: "Ingress"
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
                }
              ]
            }
          }]
        }
      }
```

Apply this newly defined TraitDefinition into our system:

```shell script
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/docs/examples/advanced-cue/combo.yaml
```

You can check it by using the application object like below:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: Application
metadata:
  name: testapp
spec:
  components:
    - name: express-server
      scopes:
        healthscopes.core.oam.dev: testapp-default-health
      settings:
        cmd:
          - node
          - server.js
        image: oamdev/testapp:v1
        port: 8080
      traits:
        - name: new-route
          properties:
            domain: test.my.domain
            http:
              "/api": 8080
      type: webservice
```

Apply it:

```shell script
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/docs/examples/advanced-cue/app1.yaml
```

Then you will see the deployment behind webservice along with the K8s service and ingress behind the new-route trait created.

### Generate multiple resources by using for loop

You can use a for-loop to generate multiple resources if you want. 
The key point is you should define the for-loop inside the outputs,
the type of parameter field used in the for-loop must be a map. 

Below is an example that will generate multiple K8s services in one trait:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  name: my-svc
spec:
  extension:
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


Apply this newly defined TraitDefinition into our system:

```shell script
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/docs/examples/advanced-cue/for-loop.yaml
```

Use the newly created trait like below:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: Application
metadata:
  name: testapp
spec:
  components:
    - name: express-server
      type: webservice
      settings:
        cmd:
          - node
          - server.js
        image: oamdev/testapp:v1
        port: 8080
      traits:
        - name: my-svc
          properties:
            http:
              myservice1: 8080
              myservice2: 8081
```

Apply it:

```shell script
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/docs/examples/advanced-cue/app2.yaml
```

Then you will see the deployment behind webservice along with two K8s services created.


## Patch Trait

For the purpose of separate of concerns, we usually won't do decomposition some fields out as trait from the underlying workload.

For example the [webservice workload] is implemented by K8s Deployment, but the workload doesn't care about the `replicas` field.
In this case, you can write a [ManualScalerTrait](https://github.com/oam-dev/kubevela/tree/master/pkg/controller/core.oam.dev/v1alpha2/core/traits/manualscalertrait)
CRD controller to control the `replicas` field after the deployment created.
 
But now, you are more encouraged to use patch trait in KubeVela. With the help of patch trait, you don't need to write CRD
controller for this case anymore.

The keyword is `patch`, object describe after the keyword will be patched into the workload.
 
Below is an example:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "Manually scale the app"
  name: scaler
spec:
  appliesToWorkloads:
    - webservice
    - worker
  extension:
    template: |-
      patch: {
         spec: replicas: parameter.replicas
      }
      parameter: {
      	replicas: *1 | int
      }
```

The patch trait rely on the workload object will always match the structure `spec.replicas` and the type of the field.
So we usually use it with the field `appliesToWorkloads` which can limit the trait can only be used by these specified workloads.

By default, the patch implemented in KubeVela relies on the CUE merge operation. It has these constraints:

* New field will be added only when the schema doesn't conflict with each other, and the value not finalized.  

For example, if the workload already define the `spec.replicas` is `5`, then the patch trait replicas value `1` will fail to patch.

* Array list in the patch will be merged into the workload by the order of index, you need to use strategy patch.


### Strategy Patch Trait

`strategy patch` is a special patch logic for patching array list supported in KubeVela, it's not native CUElang feature,
so you need to write annotation for using it.

The annotation keyword is `//+patchKey=<key_name>`.

By adding this annotation, merging logic of two array list will not follow the CUE rule, instead of that, it will
regard the element type of the array list will always be object, and compare the object field with the specified key name.
If the value of the key name equal, then the patch data will merge into that, if no equal found, the patch will append into the array list.

The example of strategy patch trait will like below:
 
```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "add sidecar to the app"
  name: sidecar
spec:
  appliesToWorkloads:
    - webservice
    - worker
  extension:
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

The patchKey is `name` which represents the container name in this example. In this case, if the workload already has
a container with the same name of this `sidecar` trait, it will be a merge operation. If the workload don't have the container
with same name, it will be a sidecar container append into the `spec.template.spec.containers` array list.

### Patch works with output

Patch can also work with output, if patch and output both exist in one trait, the patch part will execute first and then 
the output object will be rendered out. 

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "service the app"
  name: kservice
spec:
  appliesToWorkloads:
    - webservice
    - worker
  definitionRef:
    name: services
  extension:
    template: |-
      patch: {spec: template: metadata: labels: app: context.name}
      output: {
        apiVersion: "v1"
        kind: "Service"
        metadata: name: context.name
        spec: {
          selector:  app: context.name
          ports: [
            for k, v in parameter.http {
              port: v
              targetPort: v
            }
          ]
        }
      }
      parameter: {
        http: [string]: int
      }
``` 