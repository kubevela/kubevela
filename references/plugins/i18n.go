/*
Copyright 2022 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package plugins

// Language is used to define the language
type Language string

const (
	// En is English, the default language
	En Language = "English"
	// Zh is Chinese
	Zh Language = "Chinese"
)

// Definitions are all the words and phrases for internationalization in cli and docs
var Definitions = map[string]map[Language]string{
	"Description": {
		Zh: "描述",
		En: "Description",
	},
	"Samples": {
		Zh: "示例",
		En: "Samples",
	},
	"Specification": {
		Zh: "参数说明",
		En: "Specification",
	},
	"AlibabaCloud": {
		Zh: "阿里云",
		En: "Alibaba Cloud",
	},
	"AWS": {
		Zh: "AWS",
		En: "AWS",
	},
	"Azure": {
		Zh: "Azure",
		En: "Azure",
	},
	"Name": {
		Zh: "名称",
		En: "Name",
	},
	"Type": {
		Zh: "类型",
		En: "Type",
	},
	"Required": {
		Zh: "是否必须",
		En: "Required",
	},
	"Default": {
		Zh: "默认值",
		En: "Default",
	},
	"WriteConnectionSecretToRefIntroduction": {
		Zh: "如果设置了 `writeConnectionSecretToRef`，一个 Kubernetes Secret 将会被创建，并且，它的数据里有这些键（key）：",
		En: "If `writeConnectionSecretToRef` is set, a secret will be generated with these keys as below:",
	},
	"Outputs": {
		Zh: "输出",
		En: "Outputs",
	},
	"Properties": {
		Zh: "属性",
		En: "Properties",
	},
	"Terraform_configuration_for_Alibaba_Cloud_ACK_cluster": {
		Zh: "用于部署阿里云 ACK 集群的组件说明",
		En: "Terraform configuration for Alibaba Cloud ACK cluster",
	},
	"Terraform_configuration_for_Alibaba_Cloud_Serverless_Kubernetes_(ASK)": {
		Zh: "用于部署阿里云 Serverless Kubernetes (ASK) 的组件说明",
		En: "Terraform configuration for Alibaba Cloud Serverless Kubernetes (ASK)",
	},
	"Terraform_configuration_for_Alibaba_Cloud_Elastic_IP": {
		Zh: "用于部署阿里云弹性 IP 的组件说明",
		En: "Terraform configuration for Alibaba Cloud Elastic IP",
	},
	"Terraform_configuration_for_Alibaba_Cloud_OSS_object": {
		Zh: "用于部署阿里云 OSS 的组件说明",
		En: "Terraform configuration for Alibaba Cloud OSS",
	},
	"Terraform_configuration_for_Alibaba_Cloud_RDS_object": {
		Zh: "用于部署阿里云 RDS 的组件说明",
		En: "Terraform configuration for Alibaba Cloud RDS",
	},
	"Terraform_configuration_for_Alibaba_Cloud_Redis": {
		Zh: "用于部署阿里云 Redis 的组件说明",
		En: "Terraform configuration for Alibaba Cloud Redis",
	},
	"Terraform_configuration_for_Alibaba_Cloud_SLS_Project": {
		Zh: "用于部署阿里云 SLS Project 的组件说明",
		En: "Terraform configuration for Alibaba Cloud SLS Project",
	},
	"Terraform_configuration_for_Alibaba_Cloud_SLS_Store": {
		Zh: "用于部署阿里云 SLS Store 的组件说明",
		En: "Terraform configuration for Alibaba Cloud SLS Store",
	},
	"Terraform_configuration_for_Alibaba_Cloud_VPC": {
		Zh: "用于部署阿里云 VPC 的组件说明",
		En: "Terraform configuration for Alibaba Cloud VPC",
	},
	"Terraform_configuration_for_Alibaba_Cloud_VSwitch": {
		Zh: "用于部署阿里云 VSwitch 的组件说明",
		En: "Terraform configuration for Alibaba Cloud VSwitch",
	},
	"Terraform_configuration_for_AWS_S3": {
		Zh: "用于部署 AWS S3 的组件说明",
		En: "Terraform configuration for AWS S3",
	},
	"Terraform_configuration_for_Azure_Database_Mariadb": {
		Zh: "用于部署 Azure mariadb 数据库的组件说明",
		En: "Terraform configuration for Azure Database Mariadb",
	},
	"Terraform_configuration_for_Azure_Blob_Storage_Account": {
		Zh: "用于部署 Azure Blob Storage 账号的的组件说明",
		En: "Terraform configuration for Azure Blob Storage Account",
	},
}
