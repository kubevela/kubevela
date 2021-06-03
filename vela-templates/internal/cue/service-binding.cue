patch: {
	spec: template: spec: {
		// +patchKey=name
		containers: [{
			name: context.name
			// +patchKey=name
			env: [
				for envName, v in parameter.envMappings {
					name: envName
					valueFrom: {
						secretKeyRef: {
							name: v.secret
							if v["key"] != _|_ {
								key: v.key
							}
							if v["key"] == _|_ {
								key: envName
							}
						}
					}
				},
			]
		}]
	}
}

parameter: {
	// +usage=The mapping of environment variables to secret
	envMappings: [string]: [string]: string
}
