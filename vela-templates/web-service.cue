#Template: {
	apiVersion: "core.oam.dev/v1alpha2"
	kind:       "ContainerizedWorkload"
	metadata:
		name: webservice.name
	spec: {
		containers: [{
			image: webservice.image
			name:  webservice.name
			ports: [{
				containerPort: webservice.port
				protocol:      "TCP"
				name:          "default"
			}]
		}]
	}
}

webservice: {
	name: string
	// +usage=specify app image
	// +short=i
	image: string
	// +usage=specify port for container
	// +short=p
	port: *6379 | int
}
