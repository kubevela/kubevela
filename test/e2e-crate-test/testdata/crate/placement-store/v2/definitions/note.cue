"note": {
	type: "component"
	annotations: {}
	labels: {}
	description: "Placement e2e component."
	attributes: workload: definition: {
		apiVersion: "v1"
		kind:       "ConfigMap"
	}
}
template: {
	output: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: name: context.name
		data: note: parameter.message
	}
	parameter: message: *"v2" | string
}
