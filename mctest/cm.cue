config: {
	alias: ""
	annotations: {}
	attributes: workload: definition: {
		apiVersion: "v1"
		kind:       "ConfigMap"
	}
	description: ""
	labels: {}
	type: "component"
}

template: {
	output: {
		apiVersion: "v1"
        kind: "ConfigMap"
		data: {
			value: context.cluster
		}
		metadata: name: "\(context.cluster)-config-map"
	}
}
