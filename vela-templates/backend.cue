#Template: {
	apiVersion: "core.oam.dev/v1alpha2"
	kind:       "ContainerizedWorkload"
	metadata:
		name: backend.name
	spec: {
		containers: [{
			image: backend.image
			name:  backend.name
		}]
	}
}

backend: {
	name: string
	// +usage=specify app image
	// +short=i
	image: string
	// +usage=specify port for container
	// +short=p
	port: *6379 | int
}
