#deployment: {
	name: string
	// +usage=specify app image
	// +short=i
	image: string
	// +usage=specify port for container
	// +short=p
	port: *8080 | int
	env: [...{
		name:  string
		value: string
	}]
}
data: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: name: parameter.name
	spec: {
		selector:
			matchLabels:
				app: parameter.name
		template: {
			metadata:
				labels:
					app: parameter.name
			spec: containers: [{
				image: parameter.image
				name:  parameter.name
				env:   parameter.env
				ports: [{
					containerPort: parameter.port
					protocol:      "TCP"
					name:          "default"
				}]
			}]
		}
	}
}
parameter: #deployment
