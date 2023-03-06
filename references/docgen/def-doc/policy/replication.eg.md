### Background

In KubeVela, we can dispatch resources across the clusters. But projects like [OpenYurt](https://openyurt.io) have finer-grained division like node pool.
This requires to dispatch some similar resources to the same cluster. These resources are called replication. Back to the example of OpenYurt, it can
integrate KubeVela and replicate the resources then dispatch them to the different node pool.

### Usage

Replication is an internal policy. It can be only used with `deploy` workflow step. When using replication policy. A new field `replicaKey` will be added to context.
User can use definitions that make use of `context.replicaKey`. For example, apply a replica-webservice ComponentDefinition.

In this ComponentDefinition, we can use `context.replicaKey` to distinguish the name of Deployment and Service.

> **NOTE**: ComponentDefinition below is trimmed for brevity. See complete YAML in [replication.yaml](https://github.com/kubevela/kubevela/tree/master/test/e2e-test/testdata/definition/replication.yaml)

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  annotations:
    definition.oam.dev/description: Webservice, but can be replicated
  name: replica-webservice
  namespace: vela-system
spec:
  workload:
    type: autodetects.core.oam.dev
  schematic:
    cue:
      template: |
        output: {
        	apiVersion: "apps/v1"
        	kind:       "Deployment"
        	metadata: {
        		if context.replicaKey != _|_ {
        			name: context.name + "-" + context.replicaKey
        		}
        		if context.replicaKey == _|_ {
        			name: context.name
        		}
        	}
        	spec: {
        		selector: matchLabels: {
        			"app.oam.dev/component": context.name
        			if context.replicaKey != _|_ {
        				"app.oam.dev/replicaKey": context.replicaKey
        			}
        		}

        		template: {
        			metadata: {
        				labels: {
        					if parameter.labels != _|_ {
        						parameter.labels
        					}
        					if parameter.addRevisionLabel {
        						"app.oam.dev/revision": context.revision
        					}
        					"app.oam.dev/name":      context.appName
        					"app.oam.dev/component": context.name
        					if context.replicaKey != _|_ {
        						"app.oam.dev/replicaKey": context.replicaKey
        					}

        				}
        				if parameter.annotations != _|_ {
        					annotations: parameter.annotations
        				}
        			}
        		}
        	}
        }
        outputs: {
        	if len(exposePorts) != 0 {
        		webserviceExpose: {
        			apiVersion: "v1"
        			kind:       "Service"
        			metadata: {
        				if context.replicaKey != _|_ {
        					name: context.name + "-" + context.replicaKey
        				}
        				if context.replicaKey == _|_ {
        					name: context.name
        				}
        			}
        			spec: {
        				selector: {
        					"app.oam.dev/component": context.name
        					if context.replicaKey != _|_ {
        						"app.oam.dev/replicaKey": context.replicaKey
        					}
        				}
        				ports: exposePorts
        				type:  parameter.exposeType
        			}
        		}
        	}
        }
```

Then user can apply application below. Replication policy is declared in `application.spec.policies`. These policies are used in `deploy-with-rep` workflow step.
They work together to influence the `deploy` step.

- override: select `hello-rep` component to deploy.
- topology: select cluster `local` to deploy.
- replication: select `hello-rep` component to replicate.

As a result, there will be two Deployments and two Services:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: app-replication-policy
spec:
  components:
    - name: hello-rep
      type: replica-webservice
      properties:
        image: crccheck/hello-world
        ports:
          - port: 80
            expose: true
  policies:
    - name: comp-to-replicate
      type: override
      properties:
        selector: [ "hello-rep" ]
    - name: target-default
      type: topology
      properties:
        clusters: [ "local" ]
    - name: replication-default
      type: replication
      properties:
        keys: ["beijing","hangzhou"]
        selector: ["hello-rep"]

  workflow:
    steps:
      - name: deploy-with-rep
        type: deploy
        properties:
          policies: ["comp-to-replicate","target-default","replication-default"]
```

```shell
kubectl get deploy -n default
NAME                 READY   UP-TO-DATE   AVAILABLE   AGE
hello-rep-beijing    1/1     1            1           5s
hello-rep-hangzhou   1/1     1            1           5s

kubectl get service -n default
NAME                 TYPE        CLUSTER-IP     EXTERNAL-IP   PORT(S)   AGE
hello-rep-hangzhou   ClusterIP   10.43.23.200   <none>        80/TCP    41s
hello-rep-beijing    ClusterIP   10.43.24.116   <none>        80/TCP    12s
```
