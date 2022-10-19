import (
	"vela/op"
)

"print-message-in-status": {
	type: "workflow-step"
	annotations: {}
	labels: {
		"ui-hidden": "true"
	}
	description: "pring message in workflow status"
}

template: {
	parameter: {
		message: string
	}

	msg: op.#Message & {
		message: parameter.message
	}
}
