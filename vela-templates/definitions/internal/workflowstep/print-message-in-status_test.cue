import "vela/test"

"shows the message in the step status": test.#WorkflowStepExec & {
	definition: "print-message-in-status"
	parameter: message: "deployed to eu-1"
	expect: {
		phase:   "succeeded"
		message: "deployed to eu-1"
	}
}

"a message is required": test.#WorkflowStepExec & {
	definition: "print-message-in-status"
	expect: {
		phase:  "failed"
		reason: "Execute"
	}
}
