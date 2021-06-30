patch: {
	spec: template: {
		metadata: labels: {
			if parameter.type == "namespace" {
				"app.namespace.virtual.group": parameter.group
			}
			if parameter.type == "cluster" {
				"app.cluster.virtual.group": parameter.group
			}
		}
	}
}
parameter: {
	group: *"default" | string
	type:  *"namespace" | string
}
