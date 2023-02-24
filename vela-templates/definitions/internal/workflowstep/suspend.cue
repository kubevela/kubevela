import (
	"vela/op"
)

"suspend": {
	type: "workflow-step"
	annotations: {
		"category": "Process Control"
	}
	labels: {}
	description: "Suspend the current workflow, it can be resumed by 'vela workflow resume' command."
}
template: {
	suspend: op.#Suspend & {
		if parameter.duration != _|_ {
			duration: parameter.duration
		}
		if parameter.message != _|_ {
			message: parameter.message
		}
	}

	parameter: {
		// +usage=Specify the wait duration time to resume workflow such as "30s", "1min" or "2m15s"
		duration?: string
		// +usage=The suspend message to show
		message?: string
	}
}
