"suspend": {
	type: "workflow-step"
	annotations: {}
	labels: {}
	description: "Suspend your workflow"
}
template: {
	parameter: {
		// +usage=Specify the wait duration time to resume workflow such as "30s", "1min" or "2m15s"
		waitDuration: string
	}
}
