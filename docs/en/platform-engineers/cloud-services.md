# Defining Cloud Database as Component

KubeVela provides unified abstraction even for cloud services.

## Should a Cloud Service be a Component or Trait?

The following practice could be considered:
- Use `ComponentDefinition` if:
  - you want to allow your end users explicitly claim a "instance" of the cloud service and consume it, and release the "instance" when deleting the application.
- Use `TraitDefinition` if:
  - you don't want to give your end users any control/workflow of claiming or releasing the cloud service, you only want to give them a way to consume a cloud service which could even be managed by some other system. A `Service Binding` trait is widely used in this case.

In this documentation, we will add a Alibaba Cloud's RDS (Relational Database Service) as a component.

## Step 1: Install and Configure Crossplane

KubeVela uses [Crossplane](https://crossplane.io/) as the cloud service operator.

> This tutorial has been tested with Crossplane version `0.14`. Please follow the [Crossplane documentation](https://crossplane.io/docs/), especially the `Install & Configure` and `Compose Infrastructure` sections to configure
Crossplane with your cloud account.

**Note: When installing Crossplane via Helm chart, please DON'T set `alpha.oam.enabled=true` as all OAM features are already installed by KubeVela.**

## Step 2: Add Component Definition

Register the `rds` component to KubeVela.

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

Instantiate RDS component in an [Application](../application.md) to provide cloud resources.

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

Apply above application to Kubernetes and a RDS instance will be automatically provisioned (may take some time, ~5 mins).

> TBD: add status check , show database create result.


## Step 4: Consuming The Cloud Service

In this section, we will show how another component consumes the RDS instance.

> Note: we recommend to define the cloud resource claiming to an independent application if that cloud resource has standalone lifecycle. Otherwise, it could be defined in the same application of the consumer component.

### `ComponentDefinition` With Secret Reference

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
        	dbConnection: string
        	cmd?: [...string]
        }       
```

With the `//+InsertSecretTo=mySecret` annotation, KubeVela knows this parameter value comes from a Kubernetes Secret (whose name is set by user), so it will inject its data to `mySecret` which is referenced as environment variable in the template.

Then declare an application to consume the RDS instance.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: Application
metadata:
  name: data-consumer
spec:
  components:
    - name: myweb
      type: webserver
      properties:
        image: "nginx"
        dbConnection: "mydb-outputs"
```

// TBD show the result