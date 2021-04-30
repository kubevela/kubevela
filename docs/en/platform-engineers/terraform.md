---
title: Terraform
---

In addition to provisioning and consuming cloud resources by [Crossplane](./cloud-services), we can also use Terraform,
which is one of four ComponentDefinition schematic types `cue`, `kube`, `helm` and `terraform`.

To enable end users to be able to create application by Terraform, please follow these steps.

## Install Terraform Controller

Download the latest chart, like `terraform-controller-chart-0.1.4.tgz`, from the latest [releases list](https://github.com/oam-dev/terraform-controller/releases) and install it.

```shell
$ helm install terraform-controller terraform-controller-0.1.2.tgz
NAME: terraform-controller
LAST DEPLOYED: Mon Apr 26 15:55:35 2021
NAMESPACE: default
STATUS: deployed
REVISION: 1
TEST SUITE: None
```

## Apply Provider credentials

By applying Terraform Provider credentials, Terraform controller can be authenticated to deploy and manage cloud resources.

Please refer to [Terraform controller getting started](https://github.com/oam-dev/terraform-controller/blob/master/getting-started.md) on how to apply Provider for Alibaba Cloud or AWS.


## Register ComponentDefinition and TraitDefinition

### Register ComponentDefinition `alibaba-rds` as RDS cloud resource producer

Register [alibaba-rds](https://github.com/oam-dev/kubevela/tree/master/docs/examples/terraform/cloud-resource-provision-and-consume/ComponentDefinition-alibaba-rds.yaml) Component type to KubeVela.

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

### Register ComponentDefinition `alibaba-oss` as OSS cloud resource producer

Register [alibaba-oss](https://github.com/oam-dev/kubevela/tree/master/docs/examples/terraform/cloud-resource-provision-and-consume/ComponentDefinition-alibaba-oss.yaml) Component type to KubeVela.


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

### Prepare TraitDefinition `service-binding` to do env-secret mapping

Apply [service-binding](https://github.com/oam-dev/kubevela/tree/master/docs/examples/terraform/cloud-resource-provision-and-consume/TraitDefinition-service-binding.yaml) to apply service binding trait.

For more detailed introduction, please refer to [Crossplane](https://kubevela.io/docs/platform-engineers/cloud-services#prepare-traitdefinition-service-binding-to-do-env-secret-mapping).


## Next
Now You can refer to [Terraform for end users](../end-user/terraform) to provision and consume cloud resource by Terraform.
