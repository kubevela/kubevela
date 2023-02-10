import (
	"vela/op"
)

"read-object": {
	type: "workflow-step"
	annotations: {
		"category": "Resource Management"
	}
	description: "Read Kubernetes objects from cluster for your workflow steps"
}
template: {
	output: {
		if parameter.apiVersion == _|_ && parameter.kind == _|_ {
			op.#Read & {
				value: {
					apiVersion: "core.oam.dev/v1beta1"
					kind:       "Application"
					metadata: {
						name: parameter.name
						if parameter.namespace != _|_ {
							namespace: parameter.namespace
						}
					}
				}
				cluster: parameter.cluster
			}
		}
		if parameter.apiVersion != _|_ || parameter.kind != _|_ {
			op.#Read & {
				value: {
					apiVersion: parameter.apiVersion
					kind:       parameter.kind
					metadata: {
						name: parameter.name
						if parameter.namespace != _|_ {
							namespace: parameter.namespace
						}
					}
				}
				cluster: parameter.cluster
			}
		}
	}
	parameter: {
		// +usage=Specify the apiVersion of the object, defaults to 'core.oam.dev/v1beta1'
		apiVersion?: string
		// +usage=Specify the kind of the object, defaults to Application
		kind?: string
		// +usage=Specify the name of the object
		name: string
		// +usage=The namespace of the resource you want to read
		namespace?: *"default" | string
		// +usage=The cluster you want to apply the resource to, default is the current control plane cluster
		cluster: *"" | string
	}
}
