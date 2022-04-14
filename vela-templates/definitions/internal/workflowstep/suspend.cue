"suspend": {
	type: "workflow-step"
	annotations: {}
	labels: {}
	description: "Suspend your workflow"
}
template: {
	parameter: {
		// +usage=Specify the time duration string to delay such as "30s", "1min" or "2m15s"
		delayDuration: string
	}
}