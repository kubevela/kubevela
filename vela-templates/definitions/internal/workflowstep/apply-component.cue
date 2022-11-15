"apply-component": {
	type: "workflow-step"
	annotations: {}
	labels: {
		"ui-hidden": "true"
		"scope":     "Application"
	}
	description: "Apply a component and its corresbonding traits in application"
}
template: {
	parameter: {
		// +usage=Specify the component name to apply
		component: string
	}
}
