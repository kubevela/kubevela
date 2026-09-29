output: {
	type: "raw"
	properties: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: {
			name:      "echo-server"
			namespace: parameter.namespace
			labels: {
				"app.kubernetes.io/name":       "echo-server"
				"app.kubernetes.io/managed-by": "kubevela-addon"
				// Read back by the version specs, so the rendered package version
				// is observable without inspecting the registry.
				"addons.oam.dev/version": context.metadata.version
			}
		}
		spec: {
			replicas: parameter.replicas
			selector: matchLabels: "app.kubernetes.io/name": "echo-server"
			template: {
				metadata: labels: "app.kubernetes.io/name": "echo-server"
				spec: containers: [{
					name:            "echo-server"
					image:           parameter.image
					imagePullPolicy: "IfNotPresent"
					ports: [{
						name:          "http"
						containerPort: parameter.port
					}]
					env: [{
						name:  "PORT"
						value: "\(parameter.port)"
					}]
					resources: {
						requests: {
							cpu:    "50m"
							memory: "64Mi"
						}
						limits: {
							cpu:    "200m"
							memory: "128Mi"
						}
					}
				}]
			}
		}
	}
}
