scaler: {
	type: "trait"
	attributes: {
		appliesToWorkloads: ["deployments.apps"]
		status: healthPolicy: #"""
			isHealth: context.outputs.hpa.status.currentReplicas >= parameter.replicas
			"""#
	}
}
template: {
	patch: spec: replicas: parameter.replicas
	outputs: hpa: {
		apiVersion: "autoscaling/v2"
		kind:       "HorizontalPodAutoscaler"
		metadata: name: context.name
	}
	parameter: replicas: *1 | int
}
