scaler: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Manually scale K8s pod for your workload which follows the pod spec in path 'spec.template'. This definition is DEPRECATED, please use 'replication' instead."
	attributes: {
		podDisruptive: false
		appliesToWorkloads: ["*"]
	}
}
template: {
	patch: spec: replicas: parameter.replicas
	parameter: {
		// +usage=Specify the number of workload
		replicas: *1 | int
	}
}
