import (
	"vela/op"
)

"apply-application": {
	type: "workflow-step"
	annotations: {
		"category": "Application Delivery"
	}
	labels: {
		"ui-hidden":  "true"
		"deprecated": "true"
		"scope":      "Application"
	}
	description: "Apply application for your workflow steps, it has no arguments, should be used for custom steps before or after application applied."
}
template: {
	// apply application
	output: op.#ApplyApplication & {}
}
