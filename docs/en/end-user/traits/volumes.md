---
title: Cloud Volumes
---

This section introduces how to attach cloud volumes to the component. For example, AWS ElasticBlockStore,
Azure Disk, Alibaba Cloud OSS, etc.

Cloud volumes are not built-in capabilities in KubeVela so you need to enable these traits first. Let's use AWS EBS as example.

Install and check the `TraitDefinition` for AWS EBS volume trait.

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

Then we can now attach a `aws-ebs` volume to a component.
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
