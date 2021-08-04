env: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "add env into your pods"
	attributes: {
		appliesToWorkloads: ["deployments.apps"]
	}
}
template: {
	patch: {
		spec: template: spec: {
			// +patchKey=name
			containers: [{
				name: context.name
				// +patchKey=name
				env: [
					for k, v in parameter.env {
						name:  k
						value: v
					},
				]
			}]
		}
	}
	parameter: {
		env: [string]: string
	}
}
