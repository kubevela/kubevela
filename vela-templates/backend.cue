data: {
	apiVersion: "standard.oam.dev/v1alpha1"
	kind:       "Containerized"
	metadata:
		name: parameter.componentName
	spec: {
		replicas: parameter.replicas
		podSpec: {
			containers: [
				for _, c in parameter.containers {
					image: c.image
					name:  c.name
					ports: [ for _, p in c.ports {
						name:          p.name
						containerPort: p.containerPort
						protocol:      p.protocol
					}]
				}]
		}
	}
}
#backend: {
	componentName: string
	replicas:      *1 | int
	containers: [ ...{
		name: string
		// +usage=specify app image
		// +short=i
		image: string
		ports: [ ... {
			name:          *"default" | string
			containerPort: int16
			protocol:      *"TCP" | string
		}]
	}]
}
parameter: #backend
// below is a sample value
parameter: {
	componentName: "container-component"
	containers: [
		{
			name:  "c1"
			image: "image1"
			ports: [
				{
					name:          "port1"
					containerPort: 4848
					protocol:      "UDP"
				},
				{
					name:          "port2"
					containerPort: 13622
				}]
		},
		{
			name:  "c2"
			image: "image2"
			ports: [
				{
					containerPort: 8080
				}]
		},
	]
}
