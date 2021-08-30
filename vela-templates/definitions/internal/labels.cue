labels: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Add labels on K8s pod for your workload which follows the pod spec in path 'spec.template'."
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["*"]
	}
}
template: {
	patch: spec: template: metadata: labels: {
		for k, v in parameter {
			"\(k)": v
		}
	}
	parameter: [string]: string
}
