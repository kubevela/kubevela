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
			}]
		}
	}
}

#backend: {
	name: string
	// +usage=specify app image
	// +short=i
	image: string
}
parameter: #backend
