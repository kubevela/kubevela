"shop-web": {
	type: "component"
	attributes: workload: definition: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
	}
}
template: {
	output: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		spec: {
			replicas: parameter.replicas
			template: spec: containers: [{name: context.name, image: parameter.image}]
		}
	}
	parameter: {
		image:    string
		replicas: *1 | int
	}
}
