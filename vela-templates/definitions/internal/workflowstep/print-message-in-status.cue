import (
	"vela/builtin"
)

"print-message-in-status": {
	type: "workflow-step"
	annotations: {
		"category": "Process Control"
	}
	description: "print message in workflow step status"
}

template: {
	parameter: {
		message: string
	}

	msg: builtin.#Message & {
		$params: parameter
	}
}
