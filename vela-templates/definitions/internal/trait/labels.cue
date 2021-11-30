labels: {
	type: "trait"
	annotations: {}
	labels: {
		"ui-hidden": "true"
	}
	description: "Add labels on K8s pod for your workload which follows the pod spec in path 'spec.template'."
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["*"]
	}
}
template: {
	patch: {
		metadata: {
			labels: {
				for k, v in parameter {
					"\(k)": v
				}
			}
		}
		spec: template: metadata: labels: {
			for k, v in parameter {
				"\(k)": v
			}
		}
	}
	parameter: [string]: string
}
