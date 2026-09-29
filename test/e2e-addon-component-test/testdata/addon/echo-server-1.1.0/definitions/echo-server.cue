"echo-server": {
	attributes: workload: definition: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
	}
	description: "An HTTP echo server that reflects the request back to the caller."
	labels: {}
	type: "component"
}

template: {
	output: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		spec: {
			replicas: parameter.replicas
			selector: matchLabels: "app.oam.dev/component": context.name
			template: {
				metadata: labels: "app.oam.dev/component": context.name
				spec: containers: [{
					name:  context.name
					image: parameter.image
					ports: [{containerPort: parameter.port}]
				}]
			}
		}
	}

	outputs: service: {
		apiVersion: "v1"
		kind:       "Service"
		metadata: name: context.name
		spec: {
			selector: "app.oam.dev/component": context.name
			ports: [{
				port:       parameter.port
				targetPort: parameter.port
			}]
		}
	}

	parameter: {
		// +usage=Container image for the echo server
		image: *"ealen/echo-server:0.9.2" | string
		// +usage=Number of replicas
		replicas: *1 | int
		// +usage=Port the echo server listens on
		port: *80 | int
	}
}
