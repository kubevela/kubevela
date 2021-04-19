---
title: Attach Volumes
---

We will introduce how to attach basic volumes as well as extended custom
volume types for applications.

## Attach Basic Volume

`worker` and `webservice` both are capable of attaching multiple common types of
volumes, including `persistenVolumeClaim`, `configMap`, `secret`, and `emptyDir`. 
You should indicate the name of volume type in components properties. 
(we use `pvc` instead of `persistenVolumeClaim` for brevity)

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: website
spec:
  components:
    - name: frontend
      type: webservice
      properties:
        image: nginx
        volumes:
          - name: "my-pvc"    
            mountPath: "/var/www/html1" 
            type: "pvc"     # persistenVolumeClaim type volume
            claimName: "myclaim"
          - name: "my-cm"    
            mountPath: "/var/www/html2"
            type: "configMap"    # configMap type volume (specifying items)
            cmName: "myCmName"
            items:
              - key: "k1"
                path: "./a1"
              - key: "k2"
                path: "./a2"
          - name: "my-cm-noitems"
            mountPath: "/var/www/html22"
            type: "configMap"    # configMap type volume (not specifying items)
            cmName: "myCmName2"
          - name: "mysecret"
            type: "secret"     # secret type volume
            mountPath: "/var/www/html3"
            secretName: "mysecret"
          - name: "my-empty-dir"
            type: "emptyDir"    # emptyDir type volume
            mountPath: "/var/www/html4"
```

You should make sure the attached volume sources are prepared in your cluster.

## Extend custom volume types and attach

It's also allowed to extend custom volume types, such as AWS ElasticBlockStore,
Azure disk, Alibaba Cloud OSS, etc.
To enable attaching extended volume types, we should install specific Trait
capability first.

```shell
$ kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/docs/examples/app-with-volumes/td-awsEBS.yaml
```

```shell
$ kubectl vela show aws-ebs-volume
+-----------+----------------------------------------------------------------+--------+----------+---------+
|   NAME    |                          DESCRIPTION                           |  TYPE  | REQUIRED | DEFAULT |
+-----------+----------------------------------------------------------------+--------+----------+---------+
| name      | The name of volume.                                            | string | true     |         |
| mountPath |                                                                | string | true     |         |
| volumeID  | Unique id of the persistent disk resource.                     | string | true     |         |
| fsType    | Filesystem type to mount.                                      | string | true     | ext4    |
| partition | Partition on the disk to mount.                                | int    | false    |         |
| readOnly  | ReadOnly here will force the ReadOnly setting in VolumeMounts. | bool   | true     | false   |
+-----------+----------------------------------------------------------------+--------+----------+---------+
```

Then we can define an Application using aws-ebs volumes.
```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: app-worker
spec:
  components:
    - name: myworker
      type: worker
      properties:
        image: "busybox"
        cmd:
          - sleep
          - "1000"
      traits:
        - type: aws-ebs-volume
          properties:
            name: "my-ebs"
            mountPath: "/myebs"
            volumeID: "my-ebs-id"
```
