---
title: Terraform
---

在本文档中，我们将使用阿里云的 RDS（关系数据库服务）和阿里云的 OSS（对象存储系统）作为示例，展示如何在应用程序部署中启用云服务。

这些云服务由 Terraform 提供。

## 准备 Terraform 控制器

<details>

从最新的 [发布列表](https://github.com/oam-dev/terraform-controller/releases) 下载最新的图表，如 `terraform-controller-chart-0.1.8.tgz` 并安装它。

```shell
$ helm install terraform-controller terraform-controller-0.1.8.tgz
NAME: terraform-controller
LAST DEPLOYED: Mon Apr 26 15:55:35 2021
NAMESPACE: default
STATUS: deployed
REVISION: 1
TEST SUITE: None
```

### 应用提供商凭据

通过应用 Terraform Provider 凭据，可以对 Terraform 控制器进行身份验证以部署和管理云资源。

如何申请阿里云或 AWS 的 Provider 请参考[Terraform控制器入门](https://github.com/oam-dev/terraform-controller/blob/master/getting-started.md)。

</details>

### 注册`alibaba-rds`组件

注册 [alibaba-rds](https://github.com/oam-dev/kubevela/tree/master/docs/examples/terraform/cloud-resource-provision-and-consume/ComponentDefinition-alibaba-rds.yaml) 到 KubeVela。

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: ComponentDefinition
metadata:
  name: alibaba-rds
  annotations:
    definition.oam.dev/description: Terraform configuration for Alibaba Cloud RDS object
    type: terraform
spec:
  workload:
    definition:
      apiVersion: terraform.core.oam.dev/v1beta1
      kind: Configuration
  schematic:
    terraform:
      configuration: |
        module "rds" {
          source = "terraform-alicloud-modules/rds/alicloud"
          engine = "MySQL"
          engine_version = "8.0"
          instance_type = "rds.mysql.c1.large"
          instance_storage = "20"
          instance_name = var.instance_name
          account_name = var.account_name
          password = var.password
        }

        output "DB_NAME" {
          value = module.rds.this_db_instance_name
        }
        output "DB_USER" {
          value = module.rds.this_db_database_account
        }
        output "DB_PORT" {
          value = module.rds.this_db_instance_port
        }
        output "DB_HOST" {
          value = module.rds.this_db_instance_connection_string
        }
        output "DB_PASSWORD" {
          value = module.rds.this_db_instance_port
        }

        variable "instance_name" {
          description = "RDS instance name"
          type = string
          default = "poc"
        }

        variable "account_name" {
          description = "RDS instance user account name"
          type = "string"
          default = "oam"
        }

        variable "password" {
          description = "RDS instance account password"
          type = "string"
          default = "Xyfff83jfewGGfaked"
        }

```

### 注册`alibaba-oss`组件

注册 [alibaba-oss](https://github.com/oam-dev/kubevela/tree/master/docs/examples/terraform/cloud-resource-provision-and-consume/ComponentDefinition-alibaba-oss.yaml) 到 KubeVela。

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: ComponentDefinition
metadata:
  name: alibaba-oss
  annotations:
    definition.oam.dev/description: Terraform configuration for Alibaba Cloud OSS object
    type: terraform
spec:
  workload:
    definition:
      apiVersion: terraform.core.oam.dev/v1beta1
      kind: Configuration
  schematic:
    terraform:
      configuration: |
        resource "alicloud_oss_bucket" "bucket-acl" {
          bucket = var.bucket
          acl = var.acl
        }

        output "BUCKET_NAME" {
          value = "${alicloud_oss_bucket.bucket-acl.bucket}.${alicloud_oss_bucket.bucket-acl.extranet_endpoint}"
        }

        variable "bucket" {
          description = "OSS bucket name"
          default = "vela-website"
          type = string
        }

        variable "acl" {
          description = "OSS bucket ACL, supported 'private', 'public-read', 'public-read-write'"
          default = "private"
          type = string
        }


```
