annotations: {
	type: "trait"
	annotations: {}
	description: "Add annotations on your workload. If it generates pod or job, add same annotations for generated pods."
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["*"]
	}
}
template: {
	// +patchStrategy=jsonMergePatch
	patch: {
		let annotationsContent = {
			for k, v in parameter {
				(k): v
			}
		}

		metadata: {
			annotations: annotationsContent
		}
		if context.output.spec != _|_ if context.output.spec.template != _|_ {
			spec: template: metadata: annotations: annotationsContent
		}
		if context.output.spec != _|_ if context.output.spec.jobTemplate != _|_ {
			spec: jobTemplate: metadata: annotations: annotationsContent
		}
		if context.output.spec != _|_ if context.output.spec.jobTemplate != _|_ if context.output.spec.jobTemplate.spec != _|_ if context.output.spec.jobTemplate.spec.template != _|_ {
			spec: jobTemplate: spec: template: metadata: annotations: annotationsContent
		}
	}
	parameter: [string]: string | null
}
