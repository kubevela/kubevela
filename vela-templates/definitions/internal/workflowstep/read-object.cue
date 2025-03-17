import (
	"vela/kube"
)

"read-object": {
	type: "workflow-step"
	annotations: {
		"category": "Resource Management"
	}
	description: "Read Kubernetes objects from cluster for your workflow steps"
}
template: {
	output: kube.#Read & {
		$params: {
			cluster: parameter.cluster
			value: {
				apiVersion: parameter.apiVersion
				kind:       parameter.kind
				metadata: {
					name:      parameter.name
					namespace: parameter.namespace
				}
			}
		}
	}
	parameter: {
		// +usage=Specify the apiVersion of the object, defaults to 'core.oam.dev/v1beta1'
		apiVersion: *"core.oam.dev/v1beta1" | string
		// +usage=Specify the kind of the object, defaults to Application
		kind: *"Application" | string
		// +usage=Specify the name of the object
		name: string
		// +usage=The namespace of the resource you want to read
		namespace: *"default" | string
		// +usage=The cluster you want to apply the resource to, default is the current control plane cluster
		cluster: *"" | string
	}
}
