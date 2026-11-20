output: {
	type: "raw"
	properties: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: {name: "fleet-agent", namespace: "vela-system"}
		data: version: context.metadata.version
	}
}
