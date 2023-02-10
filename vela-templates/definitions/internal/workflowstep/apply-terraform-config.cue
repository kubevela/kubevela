import (
	"vela/op"
)

"apply-terraform-config": {
	alias: ""
	attributes: {}
	description: "Apply terraform configuration in the step"
	annotations: {
		"category": "Terraform"
	}
	labels: {}
	type: "workflow-step"
}

template: {
	apply: op.#Apply & {
		value: {
			apiVersion: "terraform.core.oam.dev/v1beta2"
			kind:       "Configuration"
			metadata: {
				name:      "\(context.name)-\(context.stepName)"
				namespace: context.namespace
			}
			spec: {
				deleteResource: parameter.deleteResource
				variable:       parameter.variable
				forceDelete:    parameter.forceDelete
				if parameter.source.path != _|_ {
					path: parameter.source.path
				}
				if parameter.source.remote != _|_ {
					remote: parameter.source.remote
				}
				if parameter.source.hcl != _|_ {
					hcl: parameter.source.hcl
				}
				if parameter.providerRef != _|_ {
					providerRef: parameter.providerRef
				}
				if parameter.jobEnv != _|_ {
					jobEnv: parameter.jobEnv
				}
				if parameter.writeConnectionSecretToRef != _|_ {
					writeConnectionSecretToRef: parameter.writeConnectionSecretToRef
				}
				if parameter.region != _|_ {
					region: parameter.region
				}
			}
		}
	}
	check: op.#ConditionalWait & {
		continue: apply.value.status != _|_ && apply.value.status.apply != _|_ && apply.value.status.apply.state == "Available"
	}
	parameter: {
		// +usage=specify the source of the terraform configuration
		source: close({
			// +usage=directly specify the hcl of the terraform configuration
			hcl: string
		}) | close({
			// +usage=specify the remote url of the terraform configuration
			remote: *"https://github.com/kubevela-contrib/terraform-modules.git" | string
			// +usage=specify the path of the terraform configuration
			path?: string
		})
		// +usage=whether to delete resource
		deleteResource: *true | bool
		// +usage=the variable in the configuration
		variable: {...}
		// +usage=this specifies the namespace and name of a secret to which any connection details for this managed resource should be written.
		writeConnectionSecretToRef?: {
			name:      string
			namespace: *context.namespace | string
		}
		// +usage=providerRef specifies the reference to Provider
		providerRef?: {
			name:      string
			namespace: *context.namespace | string
		}
		// +usage=region is cloud provider's region. It will override the region in the region field of providerRef
		region?: string
		// +usage=the envs for job
		jobEnv?: {...}
		// +usae=forceDelete will force delete Configuration no matter which state it is or whether it has provisioned some resources
		forceDelete: *false | bool
	}
}
