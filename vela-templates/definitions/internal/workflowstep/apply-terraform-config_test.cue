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
		phase: "failed"
		calls: "vela/kube"?: _|_
	}
} @pending(source is required but nothing reads it unless a branch field is set, so leaving it out applies a Configuration with no hcl, remote or path and the step waits)
