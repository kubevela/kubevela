outputs: service: {
	apiVersion: "v1"
	kind:       "Service"
	metadata:
		name: context.name
	spec: {
		selector:
			"app.oam.dev/component": context.name
		ports: [
			for p in parameter.http {
				port:       p
				targetPort: p
			},
		]
	}
}
parameter: {
	http: [...int]
}
