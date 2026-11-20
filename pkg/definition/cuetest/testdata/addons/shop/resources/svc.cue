output: {
	type: "raw"
	properties: {
		apiVersion: "v1"
		kind:       "Service"
		metadata: {
			name:      "shop"
			namespace: "vela-system"
			labels: version: context.metadata.version
		}
		spec: type: parameter.serviceType
	}
}
