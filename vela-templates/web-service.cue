data: {
	apiVersion: "standard.oam.dev/v1alpha1"
	kind:       "Containerized"
	metadata:
		name: parameter.name
	spec: {
		replicas: 1
		podSpec: {
			containers: [{
				image: parameter.image
				name:  parameter.name
				ports: [{
					containerPort: parameter.port
					protocol:      "TCP"
					name:          "default"
				}]
			}]
		}
	}
}
#webservice: {
	name: string
	// +usage=specify app image
	// +short=i
	image: string
	// +usage=specify port for container
	// +short=p
	port: *6379 | int
}
parameter: #webservice
