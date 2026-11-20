"fleet-agent": {
	type: "component"
	attributes: workload: definition: {
		apiVersion: "v1"
		kind:       "ConfigMap"
	}
}
template: {
	output: {apiVersion: "v1", kind: "ConfigMap", data: role: "agent"}
	parameter: {}
}
