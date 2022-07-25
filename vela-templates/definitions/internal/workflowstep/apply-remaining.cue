import (
	"vela/op"
)

"apply-remaining": {
	type: "workflow-step"
	annotations: {}
	labels: {
		"ui-hidden":  "true"
		"deprecated": "true"
	}
	description: "Apply remaining components and traits"
}
template: {
	// apply remaining components and traits
	apply: op.#ApplyRemaining & {
		parameter
	}

	parameter: {
		// +usage=Declare the name of the component
		exceptions?: [...string]
	}
}
