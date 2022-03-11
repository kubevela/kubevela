"json-patch": {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Patch the output following Json Patch strategy, following RFC 6902."
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["*"]
	}
}
template: {
	parameter: operations: [...{...}]
	// +patchStrategy=jsonPatch
	patch: parameter
}
