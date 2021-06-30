---
title: 使用 Volumes
---

我们将会介绍如何在应用中使用基本和定制化的 volumes。


## 使用基本的 Volume

`worker` 和 `webservice` 都可以使用多个通用的 volumes，包括： `persistenVolumeClaim`, `configMap`, `secret`, and `emptyDir`。你应该使用名称属性来区分不同类型的 volumes。（为了简洁，我们使用 `pvc` 代替 `persistenVolumeClaim`）

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
你需要确保使用的 volume 资源在集群中是可用的。

## 使用自定义类型的 volume

使用者可以自己扩展定制化类型的 volume，例如 AWS ElasticBlockStore，
Azure disk， Alibaba Cloud OSS。  
为了可以使用定制化类型的 volume，我们需要先安装特定的 Trait。

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

然后我们可以在应用的定义中使用 aws-ebs volumes。

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
