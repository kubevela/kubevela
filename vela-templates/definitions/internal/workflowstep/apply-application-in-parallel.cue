import (
	"vela/op"
)

"apply-application-in-parallel": {
	type: "workflow-step"
	annotations: {}
	labels: {
		"ui-hidden": "true"
	}
	description: "Apply components of an application in parallel for your workflow steps"
}
template: {
	output: op.#ApplyApplicationInParallel & {}
}
