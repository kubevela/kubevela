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
	// +patchStrategy=jsonMergePatch
	patch: {
		metadata: {
			labels: {
				for k, v in parameter {
					"\(k)": v
				}
			}
		}
		if context.output.spec != _|_ && context.output.spec.template != _|_ {
			spec: template: metadata: labels: {
				for k, v in parameter {
					"\(k)": v
				}
			}
		}
	}
	parameter: [string]: string | null
}
