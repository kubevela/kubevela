# Cloud Service

KubeVela can help you to provision and consume cloud resources with your apps very well.

## Provision

In this section, we will add a Alibaba Cloud's RDS service as a new workload type in KubeVela.

### Step 1: Install and configure Crossplane

We use [Crossplane](https://crossplane.io/) as the cloud resource operator for Kubernetes.
This tutorial has been verified with Crossplane `version 0.14`.
Please follow the Crossplane [Documentation](https://crossplane.io/docs/), 
especially the `Install & Configure` and `Compose Infrastructure` sections to configure
Crossplane with your cloud account.

**Note: When installing crossplane helm chart, please DON'T set `alpha.oam.enabled=true` as OAM crds are already installed by KubeVela.**

## Step 2: Add Component Definition

First, register the `rds` workload type to KubeVela.

```bash
$ cat << EOF | kubectl apply -f -
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: rds
  annotations:
    definition.oam.dev/apiVersion: "database.example.org/v1alpha1"
    definition.oam.dev/kind: "PostgreSQLInstance"
    definition.oam.dev/description: "RDS on Ali Cloud"
spec:
  workload:
    definition:
      apiVersion: database.example.org/v1alpha1
      kind: PostgreSQLInstance
  schematic:
    cue:
      template: |
        output: {
        	apiVersion: "database.example.org/v1alpha1"
        	kind:       "PostgreSQLInstance"
        	metadata:
        		name: context.name
        	spec: {
        		parameters:
        			storageGB: parameter.storage
        		compositionSelector: {
        			matchLabels:
        				provider: parameter.provider
        		}
        		writeConnectionSecretToRef:
        			name: parameter.secretname
        	}
        }

        parameter: {
        	secretname: *"db-conn" | string
        	provider:   *"alibaba" | string
        	storage:    *20 | int
        }
EOF
```

## Step 3: Verify

Use RDS component in an [Application](../application.md) to provide cloud resources.

As we have claimed an RDS instance with ComponentDefinition name `rds`.
The component in the application should refer to this type.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: mydatabase
spec:
  components:
    - name: myrds
      type: rds
      properties:
        name: "alibaba-rds"
        storage: 20
        secretname: "myrds-conn"
```

Apply the application into the K8s system.

The database provision will take some time (> 5 min) to be ready.

// TBD: add status check , or should database is created result.


## Consuming

In this section, we will consume the cloud resources created.

> ** Note: We highly recommend that you should split the cloud resource provision and consuming in different applications.**
** Because the cloud resources can have standalone Lifecycle Management.**
> But it also works if you combine the resources provision and consuming within an App.

### Step 1 Define a ComponentDefinition consume from secrets

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: webserver
  annotations:
    definition.oam.dev/description: "webserver to consume cloud resources"
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
        					env: [{
        						name:  "DB_NAME"
        						value: mySecret.dbName
        					}, {
        						name:  "DB_PASSWORD"
        						value: mySecret.password
        					}]
        				}]
        			}
        		}
        	}
        }
        mySecret: {
        	dbName:   string
        	password: string
        }
        parameter: {
        	image: string
        	//+InsertSecretTo=mySecret
        	mysecret: string
        	cmd?: [...string]
        }       
```

The key point is the annotation `//+InsertSecretTo=mySecret`,
KubeVela will know the parameter is a K8s secret, it will parse the secret and bind the data into the CUE struct `mySecret`.

Then the `output` can reference the `mySecret` struct for the data value. The name `mySecret` can be any name.
It's just an example in this case. The `+InsertSecretTo` is keyword, it defines the data binding mechanism.

Then create an Application to consume the data.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: Application
metadata:
  name: data-consumer
spec:
  components:
    - name: myweb
      type: webserver
      settings:
        image: "nginx"
        mysecret: "mydb-outputs"
```

// TBD show the result