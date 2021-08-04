expose: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Expose port to enable web traffic for your component."
	attributes: {
		podDisruptive: false
		appliesToWorkloads: ["deployments.apps"]
	}
}
template: {
	outputs: service: {
		apiVersion: "v1"
		kind:       "Service"
		metadata:
			name: context.name
		spec: {
			selector:
				"app.oam.dev/component": context.name
			ports: [
				for p in parameter.port {
					port:       p
					targetPort: p
				},
			]
		}
	}
	parameter: {
		// +usage=Specify the exposion ports
		port: [...int]
	}
}
