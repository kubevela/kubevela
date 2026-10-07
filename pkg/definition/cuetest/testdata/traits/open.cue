open: {
	type: "component"
	attributes: workload: definition: {
		apiVersion: "v1"
		kind:       "ConfigMap"
	}
}
template: {
	output: {apiVersion: "v1", kind: "ConfigMap"}
	outputs: _
	parameter: {}
}
