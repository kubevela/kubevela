"apply-component": {
	type: "workflow-step"
	annotations: {
		"category": "Application Delivery"
	}
	labels: {
		"scope": "Application"
	}
	description: "Apply a specific component and its corresponding traits in application"
}
template: {
	parameter: {
		// +usage=Specify the component name to apply
		component: string
		// +usage=Specify the cluster
		cluster: *"" | string
	}
}
