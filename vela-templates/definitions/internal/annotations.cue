annotations: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Add annotations on K8s pod for your workload which follows the pod spec in path 'spec.template'."
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["*"]
	}
}
template: {
	patch: {
		metadata: {
			annotations: {
				for k, v in parameter {
					"\(k)": v
				}
			}
		}
		spec: template: metadata: annotations: {
			for k, v in parameter {
				"\(k)": v
			}
		}
	}
	parameter: [string]: string
}
