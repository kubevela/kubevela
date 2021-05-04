---
title: Cloud Services
---

KubeVela allows you to declare cloud services your application needs in consistent API. Currently, we support both Terraform and Crossplane.

> Please check [the platform team guide for cloud services](../../platform-engineers/cloud-services) if you are interested in how these capabilities are maintained in KubeVela.

The cloud services will be consumed by the application via [Service Binding Trait](../traits/service-binding).

## Terraform

> ⚠️ This section assumes [Terraform related capabilities](../../platform-engineers/terraform) have been installed in your platform.

Check the parameters of cloud resource components and trait.

```shell
$ kubectl vela show alibaba-rds
# Properties
+----------------------------+-------------------------------------------------------------------------+-----------------------------------------------------------+----------+---------+
|            NAME            |                               DESCRIPTION                               |                           TYPE                            | REQUIRED | DEFAULT |
+----------------------------+-------------------------------------------------------------------------+-----------------------------------------------------------+----------+---------+
| bucket                     | OSS bucket name                                                         | string                                                    | true     |         |
| acl                        | OSS bucket ACL, supported 'private', 'public-read', 'public-read-write' | string                                                    | true     |         |
| writeConnectionSecretToRef | The secret which the cloud resource connection will be written to       | [writeConnectionSecretToRef](#writeConnectionSecretToRef) | false    |         |
+----------------------------+-------------------------------------------------------------------------+-----------------------------------------------------------+----------+---------+


## writeConnectionSecretToRef
+-----------+-----------------------------------------------------------------------------+--------+----------+---------+
|   NAME    |                                 DESCRIPTION                                 |  TYPE  | REQUIRED | DEFAULT |
+-----------+-----------------------------------------------------------------------------+--------+----------+---------+
| name      | The secret name which the cloud resource connection will be written to      | string | true     |         |
| namespace | The secret namespace which the cloud resource connection will be written to | string | false    |         |
+-----------+-----------------------------------------------------------------------------+--------+----------+---------+


$ kubectl vela show service-binding
# Properties
+-------------+------------------------------------------------+------------------+----------+---------+
|    NAME     |                  DESCRIPTION                   |       TYPE       | REQUIRED | DEFAULT |
+-------------+------------------------------------------------+------------------+----------+---------+
| envMappings | The mapping of environment variables to secret | map[string]{...} | true     |         |
+-------------+------------------------------------------------+------------------+----------+---------+
```

### Alibaba Cloud RDS and OSS

A sample [application](https://github.com/oam-dev/kubevela/tree/master/docs/examples/terraform/cloud-resource-provision-and-consume/application.yaml) is as below.

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
                secret: db-conn                                   # 1) If the env name is the same as the secret key, secret key can be omitted.
              endpoint:
                secret: db-conn
                key: DB_HOST                                      # 2) If the env name is different from secret key, secret key has to be set.
              username:
                secret: db-conn
                key: DB_USER
              # environments refer to oss-conn secret
              BUCKET_NAME:
                secret: oss-conn

    - name: sample-db
      type: alibaba-rds
      properties:
        instance_name: sample-db
        account_name: oamtest
        password: U34rfwefwefffaked
        writeConnectionSecretToRef:
          name: db-conn

    - name: sample-oss
      type: alibaba-oss
      properties:
        bucket: vela-website
        acl: private
        writeConnectionSecretToRef:
          name: oss-conn


```

## Crossplane

> ⚠️ This section assumes [Crossplane related capabilities](../../platform-engineers/crossplane) have been installed in your platform.

### Alibaba Cloud RDS and OSS

Check the parameters of cloud service component:

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
A sample application is as below.

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

## Verify

Deploy and verify the application (by either provider is OK).

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

![](../../resources/crossplane-visit-application.jpg)
