---
title: 使用 Cloud Volumes
---

本节介绍如何将 Cloud Volumes 附加到组件。 例如，AWS ElasticBlockStore、
Azure Disk、阿里云OSS等。

云卷不是 KubeVela 中的内置功能，因此您需要先启用这些特性。 让我们以 AWS EBS 为例。

安装并检查 AWS EBS volume 特征的 `TraitDefinition`。

```shell
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/docs/examples/app-with-volumes/td-awsEBS.yaml
```

```shell
kubectl vela show aws-ebs-volume
```
```console
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

然后我们现在可以将一个 `aws-ebs` volume 附加到一个组件上。

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
