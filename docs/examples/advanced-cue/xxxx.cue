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
