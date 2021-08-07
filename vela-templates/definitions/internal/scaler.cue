scaler: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Manually scale the component."
	attributes: {
		podDisruptive: false
		appliesToWorkloads: ["deployments.apps"]
	}
}
template: {
	patch: spec: replicas: parameter.replicas
	parameter: {
		// +usage=Specify the number of workload
		replicas: *1 | int
	}
}
