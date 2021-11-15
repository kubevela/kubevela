import (
	"vela/op"
)

"read-object": {
	type: "workflow-step"
	annotations: {}
	labels: {}
	description: "Read objects for your workflow steps"
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
		// +usage=Specify the apiVersion of the object, defaults to core.oam.dev/v1beta1
		apiVersion?: string
		// +usage=Specify the kind of the object, defaults to Application
		kind?: string
		// +usage=Specify the name of the object
		name: string
		// +usage=Specify the namespace of the object
		namespace?: string
		// +usage=Specify the cluster of the object
		cluster: *"" | string
	}
}
