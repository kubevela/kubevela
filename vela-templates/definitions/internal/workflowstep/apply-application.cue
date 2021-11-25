import (
	"vela/op"
)

"apply-application": {
	type: "workflow-step"
	annotations: {}
	labels: {
		"ui-hidden": "true"
	}
	description: "Apply application for your workflow steps"
}
template: {
	// apply application
	output: op.#ApplyApplication & {}
}
