output: {
	type: "raw"
	properties: {
		apiVersion: "v1"
		kind:       "Service"
		metadata: {
			name:      "echo-server"
			namespace: parameter.namespace
			labels: "app.kubernetes.io/name": "echo-server"
		}
		spec: {
			type: parameter.serviceType
			selector: "app.kubernetes.io/name": "echo-server"
			ports: [{
				name:       "http"
				port:       parameter.port
				targetPort: parameter.port
				protocol:   "TCP"
			}]
		}
	}
}
