import "vela/test"

// The Provider CRD belongs to terraform-controller; this stand-in keeps any
// fields, which is all a step that only reads one needs.
_crd: test.#Seed & {$params: objects: [{
	apiVersion: "apiextensions.k8s.io/v1"
	kind:       "CustomResourceDefinition"
	metadata: name: "providers.terraform.core.oam.dev"
	spec: {
		group: "terraform.core.oam.dev"
		scope: "Namespaced"
		names: {kind: "Provider", listKind: "ProviderList", plural: "providers", singular: "provider"}
		versions: [{
			name:    "v1beta1"
			served:  true
			storage: true
			schema: openAPIV3Schema: {type: "object", "x-kubernetes-preserve-unknown-fields": true}
			subresources: status: {}
		}]
	}
}]} @before()

// The credentials are stored as a config, through the controller's config
// factory, which a step test does not have, so #CreateConfig is always mocked.
_stored: mocks: "vela/config": "#CreateConfig": {}

// _provider is the Provider as terraform-controller would leave it.
_provider: {
	_name:  *"alibaba-provider" | string
	_state: *"ready" | string
	resources: [{
		apiVersion: "terraform.core.oam.dev/v1beta1"
		kind:       "Provider"
		metadata: name: _name
		spec: provider: "alibaba"
		status: state:  _state
	}]
}

_alibaba: {type: "alibaba", accessKey: "AKID", secretKey: "SECRET", region: "cn-hangzhou"}

"stores the credentials as a config and waits for the provider": test.#WorkflowStepExec & _stored & {
	definition: "apply-terraform-provider"
	context: namespace: "terraform-alibaba"
	parameter: _alibaba
	expect: {
		phase: "running"
		calls: {
			"vela/config": "#CreateConfig": [{$params: {
				name:      "test-app-test-step"
				namespace: "terraform-alibaba"
				template:  "terraform-alibaba"
				config: {
					name:                "alibaba-provider"
					ALICLOUD_ACCESS_KEY: "AKID"
					ALICLOUD_SECRET_KEY: "SECRET"
					ALICLOUD_REGION:     "cn-hangzhou"
				} @exact()
			}}]
			"vela/kube": "#Read": [{$params: value: {
				apiVersion: "terraform.core.oam.dev/v1beta1"
				kind:       "Provider"
				metadata: {name: "alibaba-provider", namespace: "terraform-alibaba"}
			}}]
		}
	}
}

"succeeds once the provider is ready": test.#WorkflowStepExec & _stored & _provider & {
	definition: "apply-terraform-provider"
	parameter:  _alibaba
	expect: phase: "succeeded"
}

"keeps waiting while the provider is not ready": test.#WorkflowStepExec & _stored & _provider & {
	_state:     "Initializing"
	definition: "apply-terraform-provider"
	parameter:  _alibaba
	expect: phase: "running"
}

"waits for the provider of the given name": test.#WorkflowStepExec & _stored & _provider & {
	_name:      "hangzhou"
	definition: "apply-terraform-provider"
	parameter: _alibaba & {name: "hangzhou"}
	expect: {
		phase: "succeeded"
		calls: "vela/config": "#CreateConfig": [{$params: config: name: "hangzhou"}]
	}
}

// _type runs the step for one provider type and gives the config it stores.
_type: _stored & {
	_config: {...}
	parameter: {type: string, ...}
	definition: "apply-terraform-provider"
	expect: calls: "vela/config": "#CreateConfig": [{$params: {
		template: "terraform-\(parameter.type)"
		config:   _config @exact()
	}}]
}

"stores aws credentials, with an empty session token by default": test.#WorkflowStepExec & _type & {
	parameter: {type: "aws", accessKey: "AKIA", secretKey: "SECRET", region: "eu-west-1"}
	_config: {
		name:                  "aws-provider"
		AWS_ACCESS_KEY_ID:     "AKIA"
		AWS_SECRET_ACCESS_KEY: "SECRET"
		AWS_DEFAULT_REGION:    "eu-west-1"
		AWS_SESSION_TOKEN:     ""
	}
}

"stores an aws session token": test.#WorkflowStepExec & _type & {
	parameter: {type: "aws", accessKey: "AKIA", secretKey: "SECRET", region: "eu-west-1", token: "TOKEN"}
	_config: {
		name:                  "aws-provider"
		AWS_ACCESS_KEY_ID:     "AKIA"
		AWS_SECRET_ACCESS_KEY: "SECRET"
		AWS_DEFAULT_REGION:    "eu-west-1"
		AWS_SESSION_TOKEN:     "TOKEN"
	}
}

"stores azure credentials": test.#WorkflowStepExec & _type & {
	parameter: {type: "azure", clientID: "client", clientSecret: "SECRET", subscriptionID: "sub", tenantID: "tenant"}
	_config: {
		name:                "azure-provider"
		ARM_CLIENT_ID:       "client"
		ARM_CLIENT_SECRET:   "SECRET"
		ARM_SUBSCRIPTION_ID: "sub"
		ARM_TENANT_ID:       "tenant"
	}
}

"stores baidu credentials": test.#WorkflowStepExec & _type & {
	parameter: {type: "baidu", accessKey: "AK", secretKey: "SECRET", region: "bj"}
	_config: {
		name:                  "baidu-provider"
		BAIDUCLOUD_ACCESS_KEY: "AK"
		BAIDUCLOUD_SECRET_KEY: "SECRET"
		BAIDUCLOUD_REGION:     "bj"
	}
}

"stores an elastic cloud api key": test.#WorkflowStepExec & _type & {
	parameter: {type: "ec", apiKey: "KEY"}
	_config: {name: "ec-provider", EC_API_KEY: "KEY"}
}

"stores gcp credentials": test.#WorkflowStepExec & _type & {
	parameter: {type: "gcp", credentials: "{}", region: "europe-west1", project: "shop"}
	_config: {
		name:               "gcp-provider"
		GOOGLE_CREDENTIALS: "{}"
		GOOGLE_REGION:      "europe-west1"
		GOOGLE_PROJECT:     "shop"
	}
}

"stores tencent credentials": test.#WorkflowStepExec & _type & {
	parameter: {type: "tencent", secretID: "ID", secretKey: "SECRET", region: "ap-guangzhou"}
	_config: {
		name:                    "tencent-provider"
		TENCENTCLOUD_SECRET_ID:  "ID"
		TENCENTCLOUD_SECRET_KEY: "SECRET"
		TENCENTCLOUD_REGION:     "ap-guangzhou"
	}
}

"stores ucloud credentials": test.#WorkflowStepExec & _type & {
	parameter: {type: "ucloud", publicKey: "PUB", privateKey: "PRIV", projectID: "proj", region: "cn-bj2"}
	_config: {
		name:               "ucloud-provider"
		UCLOUD_PUBLIC_KEY:  "PUB"
		UCLOUD_PRIVATE_KEY: "PRIV"
		UCLOUD_PROJECT_ID:  "proj"
		UCLOUD_REGION:      "cn-bj2"
	}
}

"the credentials of the type are required": test.#WorkflowStepExec & _stored & {
	definition: "apply-terraform-provider"
	parameter: {type: "alibaba", accessKey: "AKID"}
	expect: {
		phase:   "failed"
		message: =~"secretKey: cannot convert non-concrete value"
		calls: "vela/config"?: _|_
	}
}
