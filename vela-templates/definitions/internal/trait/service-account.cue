"service-account": {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Specify serviceAccount for your workload which follows the pod spec in path 'spec.template'."
	attributes: {
		podDisruptive: false
		appliesToWorkloads: ["*"]
	}
}
template: {
	parameter: {
		// +usage=Specify the name of ServiceAccount
		name: string
	}
	// +patchStrategy=retainKeys
	patch: spec: template: spec: serviceAccountName: parameter.name
}
