"service-binding": {
	type: "trait"
	annotations: {}
	labels: {
		"ui-hidden": "true"
	}
	description: "Binding secrets of cloud resources to component env. This definition is DEPRECATED, please use 'storage' instead."
	attributes: {
		appliesToWorkloads: ["deployments.apps", "statefulsets.apps", "daemonsets.apps", "jobs.batch"]
	}
}
template: {
	patch: spec: template: spec: {
		// +patchKey=name
		containers: [{
			name: context.name
			// +patchKey=name
			env: [
				for envName, v in parameter.envMappings {
					name: envName
					valueFrom: secretKeyRef: {
						name: v.secret
						if v["key"] != _|_ {
							key: v.key
						}
						if v["key"] == _|_ {
							key: envName
						}
					}
				},
			]
		}]
	}

	parameter: {
		// +usage=The mapping of environment variables to secret
		envMappings: [string]: #KeySecret
	}
	#KeySecret: {
		key?:   string
		secret: string
	}
}
