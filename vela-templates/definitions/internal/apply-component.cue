import (
	"vela/op"
)

"apply-component": {
	type: "workflow-step"
	annotations: {}
	labels: {}
	description: "Apply components and traits for your workflow steps"
}
template: {
	// apply components and traits
	output: op.#ApplyComponent & {
		component: parameter.component
	}

	parameter: {
		// +usage=Declare the name of the component
		component: string
	}
}
