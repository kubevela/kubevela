import (
	"vela/op"
)

"apply-object": {
	type: "workflow-step"
	annotations: {}
	labels: {}
	description: "Apply raw kubernetes objects for your workflow steps"
}
template: {
	apply: op.#Apply & {
		value: parameter
	}
	parameter: {}
}
