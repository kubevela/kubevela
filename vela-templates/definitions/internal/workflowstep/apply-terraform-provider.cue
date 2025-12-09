import (
	"vela/config"
	"vela/kube"
	"vela/builtin"
	"strings"
)

"apply-terraform-provider": {
	alias: ""
	attributes: {}
	description: "Apply terraform provider config"
	annotations: {
		"category": "Terraform"
	}
	labels: {}
	type: "workflow-step"
}

template: {
	cfg: config.#CreateConfig & {
		$params: {
			name:      "\(context.name)-\(context.stepName)"
			namespace: context.namespace
			template:  "terraform-\(parameter.type)"
			config: {
				name: parameter.name
				if parameter.type == "alibaba" {
					ALICLOUD_ACCESS_KEY: parameter.accessKey
					ALICLOUD_SECRET_KEY: parameter.secretKey
					ALICLOUD_REGION:     parameter.region
				}
				if parameter.type == "aws" {
					AWS_ACCESS_KEY_ID:     parameter.accessKey
					AWS_SECRET_ACCESS_KEY: parameter.secretKey
					AWS_DEFAULT_REGION:    parameter.region
					AWS_SESSION_TOKEN:     parameter.token
				}
				if parameter.type == "azure" {
					ARM_CLIENT_ID:       parameter.clientID
					ARM_CLIENT_SECRET:   parameter.clientSecret
					ARM_SUBSCRIPTION_ID: parameter.subscriptionID
					ARM_TENANT_ID:       parameter.tenantID
				}
				if parameter.type == "baidu" {
					BAIDUCLOUD_ACCESS_KEY: parameter.accessKey
					BAIDUCLOUD_SECRET_KEY: parameter.secretKey
					BAIDUCLOUD_REGION:     parameter.region
				}
				if parameter.type == "ec" {
					EC_API_KEY: parameter.apiKey
				}
				if parameter.type == "gcp" {
					GOOGLE_CREDENTIALS: parameter.credentials
					GOOGLE_REGION:      parameter.region
					GOOGLE_PROJECT:     parameter.project
				}
				if parameter.type == "tencent" {
					TENCENTCLOUD_SECRET_ID:  parameter.secretID
					TENCENTCLOUD_SECRET_KEY: parameter.secretKey
					TENCENTCLOUD_REGION:     parameter.region
				}
				if parameter.type == "ucloud" {
					UCLOUD_PRIVATE_KEY: parameter.privateKey
					UCLOUD_PUBLIC_KEY:  parameter.publicKey
					UCLOUD_PROJECT_ID:  parameter.projectID
					UCLOUD_REGION:      parameter.region
				}
			}
		}
	}
	read: kube.#Read & {
		$params: {
			value: {
				apiVersion: "terraform.core.oam.dev/v1beta1"
				kind:       "Provider"
				metadata: {
					name:      parameter.name
					namespace: context.namespace
				}
			}
		}
	}
	check: builtin.#ConditionalWait & {
		if read.$returns.value.status != _|_ {
			$params: continue: read.$returns.value.status.state == "ready"
		}
	}
	providerBasic: {
		accessKey!: string
		secretKey!: string
		region!:    string
	}
	#AlibabaProvider: {
		providerBasic
		type: "alibaba"
		name: *"alibaba-provider" | string
	}
	#AWSProvider: {
		providerBasic
		token: *"" | string
		type:  "aws"
		name:  *"aws-provider" | string
	}
	#AzureProvider: {
		subscriptionID: string
		tenantID:       string
		clientID:       string
		clientSecret:   string
		name:           *"azure-provider" | string
	}
	#BaiduProvider: {
		providerBasic
		type: "baidu"
		name: *"baidu-provider" | string
	}
	#ECProvider: {
		type:   "ec"
		apiKey: *"" | string
		name:   *"ec-provider" | string
	}
	#GCPProvider: {
		credentials: string
		region:      string
		project:     string
		type:        "gcp"
		name:        *"gcp-provider" | string
	}
	#TencentProvider: {
		secretID:  string
		secretKey: string
		region:    string
		type:      "tencent"
		name:      *"tencent-provider" | string
	}
	#UCloudProvider: {
		publicKey:  string
		privateKey: string
		projectID:  string
		region:     string
		type:       "ucloud"
		name:       *"ucloud-provider" | string
	}
	parameter: #AlibabaProvider | #AWSProvider | #AzureProvider | #BaiduProvider | #ECProvider | #GCPProvider | #TencentProvider | #UCloudProvider
}
