import (
	"vela/op"
)

"apply-application": {
	type: "workflow-step"
	annotations: {}
	labels: {}
	description: "Apply application for your workflow steps"
}
template: {
	// apply application
	output: op.#ApplyApplication & {}
}
