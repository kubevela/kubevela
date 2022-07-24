import (
	"vela/op"
)

"apply-object": {
	type: "workflow-step"
	annotations: {}
	labels: {
		"ui-hidden": "true"
	}
	description: "Apply raw kubernetes objects for your workflow steps"
}
template: {
	apply: op.#Apply & {
		value:   parameter.value
		cluster: parameter.cluster
	}
	parameter: {
		// +usage=Specify Kubernetes native resource object to be applied
		value: {...}
		// +usage=The cluster you want to apply the resource to, default is the current control plane cluster
		cluster: *"" | string
	}
}
