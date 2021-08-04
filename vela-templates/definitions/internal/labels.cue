labels: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Add labels for your Workload."
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["deployments.apps"]
	}
}
template: {
	patch: {
		spec: template: metadata: labels: {
			for k, v in parameter {
				"\(k)": v
			}
		}
	}
	parameter: [string]: string
}
