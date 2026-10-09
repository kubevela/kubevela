output: {
	type: "raw"
	properties: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: {
			name:      "config-store"
			namespace: parameter.namespace
			labels: {
				"app.kubernetes.io/name":       "config-store"
				"app.kubernetes.io/managed-by": "kubevela-addon"
				"addons.oam.dev/version":       context.metadata.version
			}
		}
		data: greeting: parameter.greeting
	}
}
