labels: {
	type: "trait"
	annotations: {}
	description: "Add labels on your workload. if it generates pod, add same label for generated pods."
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
					(k): v
				}
			}
		}
		if context.output.spec != _|_ && context.output.spec.template != _|_ {
			spec: template: metadata: labels: {
				for k, v in parameter {
					(k): v
				}
			}
		}
	}
	parameter: [string]: string | null
}
