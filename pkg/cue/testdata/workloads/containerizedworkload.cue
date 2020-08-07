#Template: {
	apiVersion: "core.oam.dev/v1alpha2"
	kind:       "ContainerizedWorkload"
	metadata: name: containerized.name
	spec: {
		containers: [{
			image: containerized.image
			name:  containerized.name
			ports: [{
				containerPort: containerized.port
				protocol:      "TCP"
				name:          "default"
			}]
		}]
	}
}

containerized: {
	name: string
	// +usage=specify app image
	// +short=i
	image: string
	// +usage=specify port for container
	// +short=p
	port: *6379 | int
}
