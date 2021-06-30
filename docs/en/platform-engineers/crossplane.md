---
title: Crossplane
---

In this documentation, we will use Alibaba Cloud's RDS (Relational Database Service), and Alibaba Cloud's OSS (Object Storage System) as examples to show how to enable cloud services as part of the application deployment.

These cloud services are provided by Crossplane.

## Prepare Crossplane

<details>
  
Please Refer to [Installation](https://github.com/crossplane/provider-alibaba/releases/tag/v0.5.0) 
to install Crossplane Alibaba provider v0.5.0.

> If you'd like to configure any other Crossplane providers, please refer to [Crossplane Select a Getting Started Configuration](https://crossplane.io/docs/v1.1/getting-started/install-configure.html#select-a-getting-started-configuration).

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
</details>

## Register `alibaba-rds` Component

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

## Register `alibaba-oss` Component

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
