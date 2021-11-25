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
		// +usage=Specify the value of the object
		value: {...}
		// +usage=Specify the cluster of the object
		cluster: *"" | string
	}
}
