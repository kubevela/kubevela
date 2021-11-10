scaler: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Autoscale the component based on CPU usage or scale the replica manually."
}
template: {
	patch: {
		if parameter.replicas != _|_ {
			spec: replicas: parameter.replicas
		}
	}

	outputs: {
		if parameter.auto.cpuUtil != _|_ {
			autoscaler: {
				apiVersion: "autoscaling/v1"
				kind:       "HorizontalPodAutoscaler"
				metadata: name: context.name
				spec: {
					scaleTargetRef: {
						apiVersion: parameter.auto.targetAPIVersion
						kind:       parameter.auto.targetKind
						name:       context.name
					}
					minReplicas:                    parameter.auto.min
					maxReplicas:                    parameter.auto.max
					targetCPUUtilizationPercentage: parameter.auto.cpuUtil
				}
			}
		}
	}

	parameter: {
		// +usage=Specify the number of workload manually
		replicas?: int

		auto?: {
			// +usage=Specify the minimal number of replicas to which the autoscaler can scale down
			min: *1 | int
			// +usage=Specify the maximum number of of replicas to which the autoscaler can scale up
			max: *10 | int
			// +usage=Specify the average CPU utilization, for example, 50 means the CPU usage is 50%
			cpuUtil?: int
			// +usage=Specify the apiVersion of scale target
			targetAPIVersion: *"apps/v1" | string
			// +usage=Specify the kind of scale target
			targetKind: *"Deployment" | string
		}
	}
}
