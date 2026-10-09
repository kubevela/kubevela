import "vela/test"

// The Configuration CRD belongs to terraform-controller; this stand-in keeps
// any fields, which is all a step that only applies and reads one needs.
_crd: test.#Seed & {$params: objects: [{
	apiVersion: "apiextensions.k8s.io/v1"
	kind:       "CustomResourceDefinition"
	metadata: name: "configurations.terraform.core.oam.dev"
	spec: {
		group: "terraform.core.oam.dev"
		scope: "Namespaced"
		names: {kind: "Configuration", listKind: "ConfigurationList", plural: "configurations", singular: "configuration"}
		versions: [{
			name:    "v1beta2"
			served:  true
			storage: true
			schema: openAPIV3Schema: {type: "object", "x-kubernetes-preserve-unknown-fields": true}
			subresources: status: {}
		}]
	}
}]} @before()

// The Configuration is named after the application and the step.
_name: "test-app-test-step"

_hcl: "resource \"null_resource\" \"x\" {}"

// _state is the Configuration as terraform-controller would leave it.
_state: {
	_state: string
	resources: [{
		apiVersion: "terraform.core.oam.dev/v1beta2"
		kind:       "Configuration"
		metadata: name: _name
		spec: hcl:      _hcl
		status: apply: state: _state
	}]
}

"applies the configuration and waits for it to be available": test.#WorkflowStepExec & {
	definition: "apply-terraform-config"
	parameter: {source: hcl: _hcl, variable: bucket: "shop-assets"}
	expect: {
		phase: "running"
		calls: "vela/kube": "#Apply": [{$params: value: metadata: name: _name}]
		resources: [{
			apiVersion: "terraform.core.oam.dev/v1beta2"
			kind:       "Configuration"
			metadata: name: _name
			spec: {
				hcl: _hcl
				variable: {bucket: "shop-assets"} @exact()
				deleteResource: true
				forceDelete:    false
			} @exact()
		}]
	}
}

"succeeds once the configuration is available": test.#WorkflowStepExec & _state & {
	_state:     "Available"
	definition: "apply-terraform-config"
	parameter: {source: hcl: _hcl, variable: {}}
	expect: phase: "succeeded"
}

"keeps waiting while the configuration is being provisioned": test.#WorkflowStepExec & _state & {
	_state:     "ProvisioningAndChecking"
	definition: "apply-terraform-config"
	parameter: {source: hcl: _hcl, variable: {}}
	expect: phase: "running"
}

"takes the configuration from a path in a remote repository": test.#WorkflowStepExec & {
	definition: "apply-terraform-config"
	parameter: {source: {remote: "https://github.com/acme/modules.git", path: "aws/s3"}, variable: {}}
	expect: resources: [{
		apiVersion: "terraform.core.oam.dev/v1beta2"
		kind:       "Configuration"
		metadata: name: _name
		spec: {remote: "https://github.com/acme/modules.git", path: "aws/s3", hcl?: _|_}
	}]
}

"takes the configuration from a path in the default remote repository": test.#WorkflowStepExec & {
	definition: "apply-terraform-config"
	parameter: {source: path: "aws/s3", variable: {}}
	expect: resources: [{
		apiVersion: "terraform.core.oam.dev/v1beta2"
		kind:       "Configuration"
		metadata: name: _name
		spec: {remote: "https://github.com/kubevela-contrib/terraform-modules.git", path: "aws/s3"}
	}]
}

"a source is required": test.#WorkflowStepExec & {
	definition: "apply-terraform-config"
	parameter: variable: {}
	expect: {
		phase:   "failed"
		message: "source is required: set source.hcl, or source.remote with an optional source.path"
		calls: "vela/kube"?: _|_
	}
}

"provider, connection secret, region, job env and deletion settings pass through": test.#WorkflowStepExec & {
	definition: "apply-terraform-config"
	parameter: {
		source: hcl: _hcl
		variable: {}
		deleteResource: false
		forceDelete:    true
		providerRef: {name: "aws", namespace: "vela-system"}
		writeConnectionSecretToRef: {name: "bucket-conn", namespace: "shop"}
		region: "eu-west-1"
		jobEnv: TF_LOG: "DEBUG"
	}
	expect: resources: [{
		apiVersion: "terraform.core.oam.dev/v1beta2"
		kind:       "Configuration"
		metadata: name: _name
		spec: {
			deleteResource: false
			forceDelete:    true
			providerRef: {name: "aws", namespace: "vela-system"}
			writeConnectionSecretToRef: {name: "bucket-conn", namespace: "shop"}
			region: "eu-west-1"
			jobEnv: TF_LOG: "DEBUG"
		}
	}]
}

"provider and connection secret default to the step's namespace": test.#WorkflowStepExec & {
	definition: "apply-terraform-config"
	context: namespace: "shop"
	parameter: {
		source: hcl: _hcl
		variable: {}
		providerRef: name:                "aws"
		writeConnectionSecretToRef: name: "bucket-conn"
	}
	expect: resources: [{
		apiVersion: "terraform.core.oam.dev/v1beta2"
		kind:       "Configuration"
		metadata: {name: _name, namespace: "shop"}
		spec: {
			providerRef: {name: "aws", namespace: "shop"}
			writeConnectionSecretToRef: {name: "bucket-conn", namespace: "shop"}
		}
	}]
}
