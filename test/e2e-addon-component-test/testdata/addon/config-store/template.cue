output: {
	apiVersion: "core.oam.dev/v1beta1"
	kind:       "Application"
	metadata: {
		name:      "addon-config-store"
		namespace: "vela-system"
	}
	spec: {
		components: [{
			name: "ns-config-store"
			type: "raw"
			properties: {
				apiVersion: "v1"
				kind:       "Namespace"
				metadata: name: parameter.namespace
			}
		}]
		workflow: steps: [{
			name: "apply-namespace"
			type: "apply-component"
			properties: component: "ns-config-store"
		}, {
			name: "apply-resources"
			type: "apply-remaining"
		}]
	}
}
