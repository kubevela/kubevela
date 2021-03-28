---
title:  Defining Cloud Database as Component
---

KubeVela provides unified abstraction even for cloud services.

## Should a Cloud Service be a Component or Trait?

The following practice could be considered:
- Use `ComponentDefinition` if:
  - you want to allow your end users explicitly claim a "instance" of the cloud service and consume it, and release the "instance" when deleting the application.
- Use `TraitDefinition` if:
  - you don't want to give your end users any control/workflow of claiming or releasing the cloud service, you only want to give them a way to consume a cloud service which could even be managed by some other system. A `Service Binding` trait is widely used in this case.
	
In this documentation, we will add an Alibaba Cloud's RDS (Relational Database Service), and an Alibaba Cloud's OSS (Object Storage System) as components.

## Step 1: Install and Configure Crossplane

KubeVela uses [Crossplane](https://crossplane.io/) as the cloud service operator.

> This tutorial has been tested with Crossplane version `0.14`. Please follow the [Crossplane documentation](https://crossplane.io/docs/), especially the `Install & Configure` and `Compose Infrastructure` sections to configure
Crossplane with your cloud account.

**Note: When installing Crossplane via Helm chart, please DON'T set `alpha.oam.enabled=true` as all OAM features are already installed by KubeVela.**

## Step 2: Add Component Definition

Register the `rds` component to KubeVela.

## Install and configure Crossplane

Refer to [Installation](https://github.com/crossplane/provider-alibaba/releases/tag/v0.5.0) to install Crossplane
Alibaba provider v0.5.0.

```
$ kubectl crossplane install provider crossplane/provider-alibaba:v0.5.0

$ kubectl create secret generic alibaba-account-creds -n crossplane-system --from-literal=accessKeyId=xxx --from-literal=accessKeySecret=yyy

$ kubectl apply -f provider.yaml
```

`provider.yaml` is as below.

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: crossplane-system

---
apiVersion: alibaba.crossplane.io/v1alpha1
kind: ProviderConfig
metadata:
  name: default
spec:
  credentials:
    source: Secret
    secretRef:
      namespace: crossplane-system
      name: alibaba-account-creds
      key: credentials
  region: cn-beijing
```

Note: We currently just use Crossplane Alibaba provider. But as we are about to use [Crossplane](https://crossplane.io/) as the
cloud resource operator for Kubernetes in the near future. So let's keep the following guide for future reference.

>This tutorial has been verified with Crossplane `version 0.14`.
Please follow the Crossplane [Documentation](https://crossplane.io/docs/),
especially the `Install & Configure` and `Compose Infrastructure` sections to configure
Crossplane with your cloud account.

>**Note: When installing crossplane helm chart, please DON'T set `alpha.oam.enabled=true` as OAM crds are already installed by KubeVela.**

## Provisioning and consuming cloud resource in different applications v1 (one cloud resource)

First, register the `alibaba-rds` workload type to KubeVela.

```bash
$ cat << EOF | kubectl apply -f -
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: alibaba-rds
  namespace: vela-system
  annotations:
    definition.oam.dev/description: "Alibaba Cloud RDS Resource"
spec:
  workload:
    definition:
      apiVersion: database.alibaba.crossplane.io/v1alpha1
      kind: RDSInstance
  schematic:
    cue:
      template: |
        output: {
        	apiVersion: "database.alibaba.crossplane.io/v1alpha1"
        	kind:       "RDSInstance"
        	spec: {
        		forProvider: {
        			engine:                parameter.engine
        			engineVersion:         parameter.engineVersion
        			dbInstanceClass:       parameter.instanceClass
        			dbInstanceStorageInGB: 20
        			securityIPList:        "0.0.0.0/0"
        			masterUsername:        parameter.username
        		}
        		writeConnectionSecretToRef: {
        			namespace: context.namespace
        			name:      context.outputSecretName
        		}
        		providerConfigRef: {
        			name: "default"
        		}
        		deletionPolicy: "Delete"
        	}
        }
        parameter: {
        	engine:          *"mysql" | string
        	engineVersion:   *"8.0" | string
        	instanceClass:   *"rds.mysql.c1.large" | string
        	username:        string
        }
EOF
```

Create an application with cloud resource provisioning component and consuming component as below.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: webapp
spec:
  components:
    - name: express-server
      type: webservice
      properties:
        image: zzxwill/flask-web-application:v0.3.1-crossplane
        ports: 80
      envMappings:
        DB_PASSWORD:
          secret: db-conn
          key: password                                     # 1) If the env name is different from secret key, secret key has to be set.
        endpoint:
          secret: db-conn                                   # 2) If the env name is the same as the secret key, secret key can be omitted.
        username:
          secret: db-conn
          
    - name: sample-db
      type: alibaba-rds
      properties:
        name: sample-db
        engine: mysql
        engineVersion: "8.0"
        instanceClass: rds.mysql.c1.large
        username: oamtest
        outputSecretName: db-conn
```

Apply it and verify the application.

```shell
$ kubectl get application
NAME     AGE
webapp   46m

$ kubectl get component
NAME             WORKLOAD-KIND   AGE
express-server   Deployment      23m
sample-db        RDSInstance     23m

$ sudo kubectl port-forward deployment/express-server 80:80
Password:
Forwarding from 127.0.0.1:80 -> 80
Forwarding from [::1]:80 -> 80
Handling connection for 80
Handling connection for 80
```

![](../../resources/crossplane-visit-application.jpg)



## Provisioning and consuming cloud resource in different applications

### Provision
In this section, we will add an Alibaba Cloud's RDS service as a new workload type in KubeVela.

#### Step 1: Add Component Definition

First, register the `alibaba-rds` workload type to KubeVela.

```bash
$ cat << EOF | kubectl apply -f -
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: alibaba-rds
  namespace: vela-system
  annotations:
    definition.oam.dev/description: "Alibaba Cloud RDS Resource"
spec:
  workload:
    definition:
      apiVersion: database.alibaba.crossplane.io/v1alpha1
      kind: RDSInstance
  schematic:
    cue:
      template: |
        output: {
        	apiVersion: "database.alibaba.crossplane.io/v1alpha1"
        	kind:       "RDSInstance"
        	spec: {
        		forProvider: {
        			engine:                parameter.engine
        			engineVersion:         parameter.engineVersion
        			dbInstanceClass:       parameter.instanceClass
        			dbInstanceStorageInGB: 20
        			securityIPList:        "0.0.0.0/0"
        			masterUsername:        parameter.username
        		}
        		writeConnectionSecretToRef: {
        			namespace: context.namespace
        			name:      context.outputSecretName
        		}
        		providerConfigRef: {
        			name: "default"
        		}
        		deletionPolicy: "Delete"
        	}
        }
        parameter: {
        	engine:          *"mysql" | string
        	engineVersion:   *"8.0" | string
        	instanceClass:   *"rds.mysql.c1.large" | string
        	username:        string
        }
EOF
```

#### Step 2: Verify

Instantiate RDS component in an [Application](../application.md) to provide cloud resources.

As we have claimed an RDS instance with ComponentDefinition name `alibaba-rds`. 
The component in the application should refer to this type. The yaml file `application-1-provision-cloud-service.yaml` of
the application is shown as below.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: baas-rds
spec:
  components:
    - name: sample-db
      type: alibaba-rds
      properties:
        name: sample-db
        engine: mysql
        engineVersion: "8.0"
        instanceClass: rds.mysql.c1.large
        username: oamtest
        outputSecretName: db-conn
```

Apply above application to Kubernetes and a RDS instance will be automatically provisioned (may take some time, ~5 mins).

> TBD: add status check , show database create result.


## Step 4: Consuming The Cloud Service

Apply the application into the K8s system. The database provision will take some time (> 5 min) to be ready.

A secret `db-conn` will also be created in the same namespace as that of the application.

```shell
$ kubectl apply -f application-1-provision-cloud-service.yaml

$ kubectl get application
NAME       AGE
baas-rds   9h

$ kubectl get component
NAME             WORKLOAD-KIND   AGE
sample-db        RDSInstance     9h

$ kubectl get rdsinstance
NAME           READY   SYNCED   STATE     ENGINE   VERSION   AGE
sample-db-v1   True    True     Running   mysql    8.0       9h

$ kubectl get secret
NAME                                              TYPE                                  DATA   AGE
db-conn                                           connection.crossplane.io/v1alpha1     4      9h

$ ✗ kubectl get secret db-conn -o yaml
apiVersion: v1
data:
  endpoint: xxx==
  password: yyy
  port: MzMwNg==
  username: b2FtdGVzdA==
kind: Secret
```

In this section, we will show how another component consumes the RDS instance.

### Consuming

In this section, we will show how another component consumes the RDS instance.

> Note: we recommend to define the cloud resource claiming to an independent application if that cloud resource has standalone lifecycle. Otherwise, it could be defined in the same application of the consumer component.

#### Step 1: Define a ComponentDefinition with Secret Reference

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: deployment
  annotations:
    definition.oam.dev/description: A Deployment provides declarative updates for Pods and ReplicaSets
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

  extension:
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

      					if parameter["dbSecret"] != _|_ {
      						env: [
      							{
      								name:  "username"
      								value: dbConn.username
      							},
      							{
      								name:  "endpoint"
      								value: dbConn.endpoint
      							},
      							{
      								name:  "DB_PASSWORD"
      								value: dbConn.password
      							},
      						]
      					}

      					ports: [{
      						containerPort: parameter.port
      					}]

      					if parameter["cpu"] != _|_ {
      						resources: {
      							limits:
      								cpu: parameter.cpu
      							requests:
      								cpu: parameter.cpu
      						}
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

      	// +usage=Commands to run in the container
      	cmd?: [...string]

      	// +usage=Which port do you want customer traffic sent to
      	// +short=p
      	port: *80 | int

      	// +usage=Referred db secret
      	// +insertSecretTo=dbConn
      	dbSecret?: string

      	// +usage=Number of CPU units for the service, like `0.5` (0.5 CPU core), `1` (1 CPU core)
      	cpu?: string
      }

      dbConn: {
      	username: string
      	endpoint: string
      	port:     string
      }     
```

The key point is the annotation `//+insertSecretTo=dbConn`,
KubeVela will know the parameter is a K8s secret, it will parse the secret and bind the data into the CUE struct `dbConn`.

Then the `output` can reference the `dbConn` struct for the data value. The name `dbConn` can be any name.
It's just an example in this case. The `+insertSecretTo` is keyword, it defines the data binding mechanism.

The application yaml `application-2-consume-cloud-resource.yaml` is shown as below. 

Then create the Application to consume the data

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: webapp
spec:
  components:
    - name: express-server
      type: deployment
      properties:
        image: zzxwill/flask-web-application:v0.3.1-crossplane
        ports: 80
        dbSecret: db-conn
```

```shell
$ kubectl apply -f application-2-consume-cloud-resource.yaml

$ kubectl get application
NAME       AGE
baas-rds   10h
webapp     14h

$ kubectl get deployment
NAME                READY   UP-TO-DATE   AVAILABLE   AGE
express-server-v1   1/1     1            1           9h

$ kubectl port-forward deployment/express-server-v1 80:80
```

We can see the cloud resource is successfully consumed by the application.

![](../../resources/crossplane-visit-application.jpg)
