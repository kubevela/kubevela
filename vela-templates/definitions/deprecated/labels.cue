labels: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Add labels on K8s pod for your workload which follows the pod spec in path 'spec.template'. This definition is DEPRECATED, please specify labels in component instead."
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
