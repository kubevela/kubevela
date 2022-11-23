import (
	"vela/op"
)

"print-message-in-status": {
	type: "workflow-step"
	annotations: {}
	labels: {
		"ui-hidden": "true"
	}
	description: "print message in workflow step status"
}

template: {
	parameter: {
		message: string
	}

	msg: op.#Message & {
		message: parameter.message
	}
}
