app: {
	type: "component"
	attributes: {
		workload: definition: {
			apiVersion: "apps/v1"
			kind:       "Deployment"
		}
		status: healthPolicy: #"""
			isHealth: *(context.output.status.readyReplicas > 0) | false
			"""#
	}
}
template: {
	output: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		spec: template: spec: containers: [{name: context.name, image: parameter.image}]
	}
	outputs: service: {
		apiVersion: "v1"
		kind:       "Service"
		metadata: name: context.name
	}
	parameter: image: string
}
