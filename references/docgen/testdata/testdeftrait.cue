testdeftrait: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Add host aliases on K8s pod for your workload which follows the pod spec in path 'spec.template'."
	attributes: {
		podDisruptive: false
		appliesToWorkloads: ["*"]
	}
}
template: {
	patch: {
		// +patchKey=ip
		spec: template: spec: hostAliases: parameter.hostAliases
	}
	parameter: {
		// +usage=Specify the hostAliases to add
		hostAliases: [...{
			ip: string
			hostnames: [...string]
		}]
	}
}
