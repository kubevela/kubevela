svc: {
	type: "trait"
	attributes: appliesToWorkloads: ["*"]
}
template: {
	outputs: service: {
		apiVersion: "v1"
		kind:       "Service"
		metadata: {
			name: context.name
			annotations: from: "svc"
		}
	}
	parameter: {}
}
