#Template: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: name: deployment.name
	spec: {
		containers: [{
			image: deployment.image
			name:  deployment.name
			env:   deployment.env
			ports: [{
				containerPort: deployment.port
				protocol:      "TCP"
				name:          "default"
			}]
		}]
	}
}

deployment: {
	name:  string
	image: string
	port:  *8080 | int
	env: [...{
		name:  string
		value: string
	}]
}
