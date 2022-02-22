patch: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Patch the output directly."
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["*"]
	}
}
template: {
	parameter: {...}
	// +patchStrategy=open
	patch: parameter
}
