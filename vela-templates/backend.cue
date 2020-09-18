#Template: {
	apiVersion: "standard.oam.dev/v1alpha1"
	kind:       "Containerized"
	metadata:
		name: backend.name
	spec: {
		replicas: 1
		podSpec: {
			containers: [{
				image: backend.image
				name:  backend.name
			}]
		}
	}
}

backend: {
	name: string
	// +usage=specify app image
	// +short=i
	image: string
}
