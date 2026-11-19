output: {
	apiVersion: "core.oam.dev/v1beta1"
	kind:       "Application"
	metadata: {
		name:      "addon-echo-server"
		namespace: "vela-system"
	}
	spec: {
		components: [{
			name: "ns-echo-server"
			type: "raw"
			properties: {
				apiVersion: "v1"
				kind:       "Namespace"
				metadata: name: parameter.namespace
			}
		}]
		// The namespace must exist before the Deployment and Service land in it.
		workflow: steps: [{
			name: "apply-namespace"
			type: "apply-component"
			properties: component: "ns-echo-server"
		}, {
			name: "apply-resources"
			type: "apply-remaining"
		}]
	}
}
