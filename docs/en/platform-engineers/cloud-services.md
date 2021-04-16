---
title: Crossplane
---

Cloud services is also part of your application deployment.

## Should a Cloud Service be a Component or Trait?

The following practice could be considered:
- Use `ComponentDefinition` if:
  - you want to allow your end users explicitly claim a "instance" of the cloud service and consume it, and release the "instance" when deleting the application.
- Use `TraitDefinition` if:
  - you don't want to give your end users any control/workflow of claiming or releasing the cloud service, you only want to give them a way to consume a cloud service which could even be managed by some other system. A `Service Binding` trait is widely used in this case.
	
In this documentation, we will define an Alibaba Cloud's RDS (Relational Database Service), and an Alibaba Cloud's OSS (Object Storage System) as example. This mechanism works the same with other cloud providers.
In a single application, they are in form of Traits, and in multiple applications, they are in form of Components.

## Install and Configure Crossplane

This guide will use [Crossplane](https://crossplane.io/) as the cloud service provider. Please Refer to [Installation](https://github.com/crossplane/provider-alibaba/releases/tag/v0.5.0) 
to install Crossplane Alibaba provider v0.5.0.

If you'd like to configure any other Crossplane providers, please refer to [Crossplane Select a Getting Started Configuration](https://crossplane.io/docs/v1.1/getting-started/install-configure.html#select-a-getting-started-configuration).

```
$ kubectl crossplane install provider crossplane/provider-alibaba:v0.5.0

# Note the xxx and yyy here is your own AccessKey and SecretKey to the cloud resources.
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

Note: We currently just use Crossplane Alibaba provider. But we are about to use [Crossplane](https://crossplane.io/) as the
cloud resource operator for Kubernetes in the near future.

## Register ComponentDefinition and TraitDefinition

### Register ComponentDefinition `alibaba-rds` as RDS cloud resource producer

Register the `alibaba-rds` workload type to KubeVela.

```yaml
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
        			name:      parameter.secretName
        		}
        		providerConfigRef: {
        			name: "default"
        		}
        		deletionPolicy: "Delete"
        	}
        }
        parameter: {
        	// +usage=RDS engine
        	engine: *"mysql" | string
        	// +usage=The version of RDS engine
        	engineVersion: *"8.0" | string
        	// +usage=The instance class for the RDS
        	instanceClass: *"rds.mysql.c1.large" | string
        	// +usage=RDS username
        	username: string
        	// +usage=Secret name which RDS connection will write to
        	secretName: string
        }


```

### Register ComponentDefinition `alibaba-oss` as OSS cloud resource producer

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: alibaba-oss
  namespace: vela-system
  annotations:
    definition.oam.dev/description: "Alibaba Cloud RDS Resource"
spec:
  workload:
    definition:
      apiVersion: oss.alibaba.crossplane.io/v1alpha1
      kind: Bucket
  schematic:
    cue:
      template: |
        output: {
        	apiVersion: "oss.alibaba.crossplane.io/v1alpha1"
        	kind:       "Bucket"
        	spec: {
        		name:               parameter.name
        		acl:                parameter.acl
        		storageClass:       parameter.storageClass
        		dataRedundancyType: parameter.dataRedundancyType
        		writeConnectionSecretToRef: {
        			namespace: context.namespace
        			name:      parameter.secretName
        		}
        		providerConfigRef: {
        			name: "default"
        		}
        		deletionPolicy: "Delete"
        	}
        }
        parameter: {
        	// +usage=OSS bucket name
        	name: string
        	// +usage=The access control list of the OSS bucket
        	acl: *"private" | string
        	// +usage=The storage type of OSS bucket
        	storageClass: *"Standard" | string
        	// +usage=The data Redundancy type of OSS bucket
        	dataRedundancyType: *"LRS" | string
        	// +usage=Secret name which RDS connection will write to
        	secretName: string
        }

```

### Register ComponentDefinition `webconsumer` with Secret Reference

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: webconsumer
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
        	password: string
        }

```

The key point is the annotation `//+insertSecretTo=dbConn`, KubeVela will know the parameter is a K8s secret, it will parse
the secret and bind the data into the CUE struct `dbConn`.

Then the `output` can reference the `dbConn` struct for the data value. The name `dbConn` can be any name.
It's just an example in this case. The `+insertSecretTo` is keyword, it defines the data binding mechanism.

### Prepare TraitDefinition `service-binding` to do env-secret mapping

As for data binding in Application, KubeVela recommends defining a trait to finish the job. We have prepared a common
trait for convenience. This trait works well for binding resources' info into pod spec Env.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "binding cloud resource secrets to pod env"
  name: service-binding
spec:
  appliesToWorkloads:
    - webservice
    - worker
  schematic:
    cue:
      template: |
        patch: {
        	spec: template: spec: {
        		// +patchKey=name
        		containers: [{
        			name: context.name
        			// +patchKey=name
        			env: [
        				for envName, v in parameter.envMappings {
        					name: envName
        					valueFrom: {
        						secretKeyRef: {
        							name: v.secret
        							if v["key"] != _|_ {
        								key: v.key
        							}
        							if v["key"] == _|_ {
        								key: envName
        							}
        						}
        					}
        				},
        			]
        		}]
        	}
        }

        parameter: {
        	// +usage=The mapping of environment variables to secret
        	envMappings: [string]: [string]: string
        }

```

With the help of this `service-binding` trait, developers can explicitly set parameter `envMappings` to mapping all
environment names with secret key. Here is an example.

```yaml
...
      traits:
        - type: service-binding
          properties:
            envMappings:
              # environments refer to db-conn secret
              DB_PASSWORD:
                secret: db-conn
                key: password                                     # 1) If the env name is different from secret key, secret key has to be set.
              endpoint:
                secret: db-conn                                   # 2) If the env name is the same as the secret key, secret key can be omitted.
              username:
                secret: db-conn
              # environments refer to oss-conn secret
              BUCKET_NAME:
                secret: oss-conn
                key: Bucket
...
```

You can see [the end user usage workflow](../end-user/cloud-resources) to know how it used. 