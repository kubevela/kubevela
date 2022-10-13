expose: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Expose a service."
	attributes: podDisruptive: false
}
template: {
	parameter: {
		domain: string
		http: [string]: int
	}
	outputs: {
		for k, v in parameter.http {
			(k): {
				apiVersion: "v1"
				kind:       "Service"
				spec: {
					selector: app: context.name
					ports: [{
						port:       v
						targetPort: v
					}]
				}
			}
		}
	}
}
