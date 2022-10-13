annotations: {
	type: "trait"
	annotations: {}
	description: "Add annotations on your workload. if it generates pod, add same annotations for generated pods."
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["*"]
	}
}
template: {
	// +patchStrategy=jsonMergePatch
	patch: {
		metadata: {
			annotations: {
				for k, v in parameter {
					(k): v
				}
			}
		}
		if context.output.spec != _|_ && context.output.spec.template != _|_ {
			spec: template: metadata: annotations: {
				for k, v in parameter {
					(k): v
				}
			}
		}
	}
	parameter: [string]: string | null
}
