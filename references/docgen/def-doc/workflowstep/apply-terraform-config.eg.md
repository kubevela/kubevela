```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: apply-terraform-resource
  namespace: default
spec:
  components: []
  workflow:
    steps:
    - name: provider
      type: apply-terraform-provider
      properties:
        type: alibaba
        name: my-alibaba-provider
        accessKey: <accessKey>
        secretKey: <secretKey>
        region: cn-hangzhou
    - name: configuration
      type: apply-terraform-config
      properties:
        source:
          path: alibaba/cs/dedicated-kubernetes
          remote: https://github.com/FogDong/terraform-modules
        providerRef:
          name: my-alibaba-provider
        writeConnectionSecretToRef:
            name: my-terraform-secret
            namespace: vela-system
        variable:
          name: regular-check-ack
          new_nat_gateway: true
          vpc_name: "tf-k8s-vpc-regular-check"
          vpc_cidr: "10.0.0.0/8"
          vswitch_name_prefix: "tf-k8s-vsw-regualr-check"
          vswitch_cidrs: [ "10.1.0.0/16", "10.2.0.0/16", "10.3.0.0/16" ]
          k8s_name_prefix: "tf-k8s-regular-check"
          k8s_version: 1.24.6-aliyun.1
          k8s_pod_cidr: "192.168.5.0/24"
          k8s_service_cidr: "192.168.2.0/24"
          k8s_worker_number: 2
          cpu_core_count: 4
          memory_size: 8
          tags:
            created_by: "Terraform-of-KubeVela"
            created_from: "module-tf-alicloud-ecs-instance"
```