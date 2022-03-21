"test-stateful": {
	annotations: {}
	attributes: workload: definition: {
		apiVersion: "apps/v1"
		kind:       "StatefulSet"
	}
	description: "StatefulSet component."
	labels: {}
	type: "component"
}

template: {
	output: {
		apiVersion: "apps/v1"
		kind:       "StatefulSet"
		metadata: name: context.name
		spec: {
			selector: matchLabels: app: context.name
			minReadySeconds: 10
			replicas:        parameter.replicas
			serviceName:     context.name
			template: {
				metadata: labels: app: context.name
				spec: {
					containers: [{
						name: "nginx"
						ports: [{
							name:          "web"
							containerPort: 80
						}]
						image: parameter.image
					}]
					terminationGracePeriodSeconds: 10
				}
			}
		}
	}
	outputs: web: {
		apiVersion: "v1"
		kind:       "Service"
		metadata: {
			name: context.name
			labels: app: context.name
		}
		spec: {
			clusterIP: "None"
			ports: [{
				name: "web"
				port: 80
			}]
			selector: app: context.name
		}
	}
	parameter: {
		image:    string
		replicas: int
	}
}
