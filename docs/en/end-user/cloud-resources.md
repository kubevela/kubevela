---
title: Provision and Consume Cloud Resources
---

> ⚠️ This section requires your platform builder has already installed the [cloud resources related capabilities](../platform-engineers/cloud-services).

## Provision and consume cloud resource in a single application v1 (one cloud resource)

Check the parameters of cloud resource component:

```shell
$ kubectl vela show alibaba-rds

# Properties
+---------------+------------------------------------------------+--------+----------+--------------------+
|     NAME      |                  DESCRIPTION                   |  TYPE  | REQUIRED |      DEFAULT       |
+---------------+------------------------------------------------+--------+----------+--------------------+
| engine        | RDS engine                                     | string | true     | mysql              |
| engineVersion | The version of RDS engine                      | string | true     |                8.0 |
| instanceClass | The instance class for the RDS                 | string | true     | rds.mysql.c1.large |
| username      | RDS username                                   | string | true     |                    |
| secretName    | Secret name which RDS connection will write to | string | true     |                    |
+---------------+------------------------------------------------+--------+----------+--------------------+
```

Use the service binding trait to bind cloud resources into workload as ENV.

Create an application with a cloud resource provisioning component and a consuming component as below.

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

    - name: sample-db
      type: alibaba-rds
      properties:
        name: sample-db
        engine: mysql
        engineVersion: "8.0"
        instanceClass: rds.mysql.c1.large
        username: oamtest
        secretName: db-conn

```

Apply it and verify the application.

```shell
$ kubectl get application
NAME     AGE
webapp   46m

$ kubectl port-forward deployment/express-server 80:80
Forwarding from 127.0.0.1:80 -> 80
Forwarding from [::1]:80 -> 80
Handling connection for 80
Handling connection for 80
```

![](../resources/crossplane-visit-application.jpg)

## Provision and consume cloud resource in a single application v2 (two cloud resources)

Based on the section `Provisioning and consuming cloud resource in a single application v1 (one cloud resource)`, 

Update the application to also consume cloud resource OSS.

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

    - name: sample-db
      type: alibaba-rds
      properties:
        name: sample-db
        engine: mysql
        engineVersion: "8.0"
        instanceClass: rds.mysql.c1.large
        username: oamtest
        secretName: db-conn

    - name: sample-oss
      type: alibaba-oss
      properties:
        name: velaweb
        secretName: oss-conn
```

Apply it and verify the application.

```shell
$ kubectl port-forward deployment/express-server 80:80
Forwarding from 127.0.0.1:80 -> 80
Forwarding from [::1]:80 -> 80
Handling connection for 80
Handling connection for 80
```

![](../resources/crossplane-visit-application-v2.jpg)

## Provision and consume cloud resource in different applications

In this section, cloud resource will be provisioned in one application and consumed in another application.

### Provision Cloud Resource

Instantiate RDS component with `alibaba-rds` workload type in an [Application](../application) to provide cloud resources.

As we have claimed an RDS instance with ComponentDefinition name `alibaba-rds`.
The component in the application should refer to this type.

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
        secretName: db-conn
```

Apply the application to Kubernetes and a RDS instance will be automatically provisioned (may take some time, ~2 mins).

A secret `db-conn` will also be created in the same namespace as that of the application.

```shell
$ kubectl get application
NAME       AGE
baas-rds   9h

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

### Consume the Cloud Resource

In this section, we will show how another component consumes the RDS instance.

> Note: we recommend defining the cloud resource claiming to an independent application if that cloud resource has
> standalone lifecycle.

Now create the Application to consume the data.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: webapp
spec:
  components:
    - name: express-server
      type: webconsumer
      properties:
        image: zzxwill/flask-web-application:v0.3.1-crossplane
        ports: 80
        dbSecret: db-conn
```

```shell
$ kubectl get application
NAME       AGE
baas-rds   10h
webapp     14h

$ kubectl get deployment
NAME                READY   UP-TO-DATE   AVAILABLE   AGE
express-server-v1   1/1     1            1           9h

$ kubectl port-forward deployment/express-server 80:80
```

We can see the cloud resource is successfully consumed by the application.

![](../resources/crossplane-visit-application.jpg)
