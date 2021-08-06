annotations: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Add annotations for your Workload."
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["deployments.apps"]
	}
}
template: {
	patch: spec: template: metadata: annotations: {
		for k, v in parameter {
			"\(k)": v
		}
	}
	parameter: [string]: string
}
