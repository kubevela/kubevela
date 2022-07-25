"suspend": {
	type: "workflow-step"
	annotations: {}
	labels: {}
	description: "Suspend the current workflow, it can be resumed by 'vela workflow resume' command."
}
template: {
	parameter: {
		// +usage=Specify the wait duration time to resume workflow such as "30s", "1min" or "2m15s"
		duration?: string
	}
}
