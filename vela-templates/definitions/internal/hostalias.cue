hostalias: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Add host aliases to workloads."
	attributes: {
		podDisruptive: false
		appliesToWorkloads: ["deployment.apps"]
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
