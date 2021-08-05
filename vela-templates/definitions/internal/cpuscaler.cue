cpuscaler: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Automatically scale the component based on CPU usage."
	attributes: appliesToWorkloads: ["deployments.apps"]
}
template: {
	outputs: cpuscaler: {
		apiVersion: "autoscaling/v2beta2"
		kind:       "HorizontalPodAutoscaler"
		metadata: name: context.name
		spec: {
			scaleTargetRef: {
				apiVersion: "apps/v1"
				kind:       "Deployment"
				name:       context.name
			}
			minReplicas: parameter.min
			maxReplicas: parameter.max
			metrics: [{
				type: "Resource"
				resource: {
					name: "cpu"
					target: {
						type:               "Utilization"
						averageUtilization: parameter.cpuUtil
					}
				}
			}]
		}
	}
	parameter: {

		// +usage=Specify the minimal number of replicas to which the autoscaler can scale down
		min: *1 | int

		// +usage=Specify the maximum number of of replicas to which the autoscaler can scale up
		max: *10 | int

		// +usage=Specify the average cpu utilization, for example, 50 means the CPU usage is 50%
		cpuUtil: *50 | int
	}
}
