# Cloud Service

In this tutorial, we will add a Alibaba Cloud's RDS service as a new workload type in KubeVela.

## Step 1: Install and configure Crossplane

We use Crossplane as the cloud resource operator for Kubernetes. This tutorial has been verified with Crossplane `version 0.14`.
Please follow the Crossplane [Documentation](https://crossplane.io/docs/), especially the `Install & Configure` and `Compose Infrastructure` sections to configure Crossplane with your cloud account.

**Note: When installing crossplane helm chart, please don't set `alpha.oam.enabled=true` as OAM crds are already installed by KubeVela.**

## Step 2: Add Workload Definition

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

Check if the new workload type is added:

```console
$ vela workloads
Synchronizing capabilities from cluster⌛ ...
Sync capabilities successfully ✅ Add(1) Update(0) Delete(0)
TYPE	CATEGORY	DESCRIPTION     
+rds	workload	RDS on Ali Cloud

Listing workload capabilities ...

NAME      	DESCRIPTION                             
rds       	RDS on Ali Cloud                        
task      	One-time task/job                       
webservice	Long running service with network routes
worker    	Backend worker without ports exposed    
```

(Optional) Define RDS component in an application

<details>

Let's first create an [Appfile](../developers/learn-appfile.md). We will claim an RDS instance with workload type of `rds`. You may need to change the variables of the `database` service to reflect your configuration.

```bash
$ cat << EOF > vela.yaml
name: test-rds

services:
  database:
    type: rds
    name: alibabaRds
    storage: 20

  checkdb:
    type: webservice
    image: nginx
    name: checkdb
    env:
      - name: PGDATABASE
        value: postgres
      - name: PGHOST
        valueFrom:
          secretKeyRef:
            name: db-conn
            key: endpoint
      - name: PGUSER
        valueFrom:
          secretKeyRef:
            name: db-conn
            key: username
      - name: PGPASSWORD
        valueFrom:
          secretKeyRef:
            name: db-conn
            key: password
      - name: PGPORT
        valueFrom:
          secretKeyRef:
            name: db-conn
            key: port
EOF
```

Next, we could deploy the application with `$ vela up`.

## Verify the database status

The database provision will take some time (> 5 min) to be ready.
In our Appfile, we created another service called `checkdb`. The database will write all the connecting credentials in a secret which we put into the `checkdb` service as environmental variables. To verify the database configuration, we simply print out the environmental variables of the `checkdb` service:   
`$ vela exec test-rds -- printenv`   
After confirming the service is `checkdb`, we shall see the printout of the database information:

```console
PGUSER=myuser
PGPASSWORD=<password>
PGPORT=1921
PGDATABASE=postgres
PGHOST=<hostname>
...
```
</details>

