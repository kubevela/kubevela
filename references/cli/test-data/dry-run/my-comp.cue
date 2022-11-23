"my-comp": {
	annotations: {}
	attributes: workload: definition: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
	}
	description: "My component."
	labels: {}
	type: "component"
}
template: {
	output: {
		metadata: name: "hello-world"
		spec: {
			replicas: 1
			selector: matchLabels: "app.kubernetes.io/name": "hello-world"
			template: {
				metadata: labels: "app.kubernetes.io/name": "hello-world"
				spec: containers: [{
					name:  "hello-world"
					image: "somefive/hello-world"
					ports: [{
						name:          "http"
						containerPort: 80
						protocol:      "TCP"
					}]
				}]
			}
		}
		apiVersion: "apps/v1"
		kind:       "Deployment"
	}
	outputs: "hello-world-service": {
		metadata: name: "hello-world-service"
		spec: {
			ports: [{
				name:       "http"
				protocol:   "TCP"
				port:       80
				targetPort: 8080
			}]
			selector: app: "hello-world"
			type: "LoadBalancer"
		}
		apiVersion: "v1"
		kind:       "Service"
	}
	parameter: {}

}
