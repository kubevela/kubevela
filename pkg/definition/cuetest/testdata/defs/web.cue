web: {
	type: "component"
	attributes: {
		workload: definition: {
			apiVersion: "apps/v1"
			kind:       "Deployment"
		}
		status: {
			healthPolicy: #"""
				isHealth: context.output.status.readyReplicas == context.output.spec.replicas
				"""#
			customStatus: #"""
				message: "Ready:\(context.output.status.readyReplicas)/\(context.output.spec.replicas)"
				"""#
			details: #"""
				image: parameter.image
				"""#
		}
	}
}
template: {
	output: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: context.name
		spec: {
			replicas: parameter.replicas
			template: spec: containers: [{image: parameter.image}]
		}
	}
	if parameter.expose {
		outputs: service: {
			apiVersion: "v1"
			kind:       "Service"
			metadata: name: context.name
		}
	}
	parameter: {
		image:    string
		replicas: *1 | int
		expose:   *false | bool
	}
}
