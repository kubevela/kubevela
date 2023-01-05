hpa: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Configure k8s HPA for Deployment or Statefulsets"
	attributes: {
		podDisruptive: false
		appliesToWorkloads: ["deployments.apps", "statefulsets.apps"]
	}
}
template: {
	outputs: hpa: {
		if context.clusterVersion.minor < 23 {
			apiVersion: "autoscaling/v2beta2"
		}
		if context.clusterVersion.minor >= 23 {
			apiVersion: "autoscaling/v2"
		}
		kind: "HorizontalPodAutoscaler"
		metadata: name: context.name
		spec: {
			scaleTargetRef: {
				apiVersion: parameter.targetAPIVersion
				kind:       parameter.targetKind
				name:       context.name
			}
			minReplicas: parameter.min
			maxReplicas: parameter.max
			metrics: [
				{
					type: "Resource"
					resource: {
						name: "cpu"
						target: {
							type: parameter.cpu.type
							if parameter.cpu.type == "Utilization" {
								averageUtilization: parameter.cpu.value
							}
							if parameter.cpu.type == "AverageValue" {
								averageValue: parameter.cpu.value
							}
						}
					}
				},
				if parameter.mem != _|_ {
					{
						type: "Resource"
						resource: {
							name: "memory"
							target: {
								type: parameter.mem.type
								if parameter.cpu.type == "Utilization" {
									averageUtilization: parameter.cpu.value
								}
								if parameter.cpu.type == "AverageValue" {
									averageValue: parameter.cpu.value
								}
							}
						}
					}
				},
				if parameter.podCustomMetrics != _|_ for m in parameter.podCustomMetrics {
					type: "Pods"
					pods: {
						metric: {
							name: m.name
						}
						target: {
							type:         "AverageValue"
							averageValue: m.value
						}
					}
				},
			]
		}
	}
	parameter: {
		// +usage=Specify the minimal number of replicas to which the autoscaler can scale down
		min: *1 | int
		// +usage=Specify the maximum number of of replicas to which the autoscaler can scale up
		max: *10 | int
		// +usage=Specify the apiVersion of scale target
		targetAPIVersion: *"apps/v1" | string
		// +usage=Specify the kind of scale target
		targetKind: *"Deployment" | string
		cpu: {
			// +usage=Specify resource metrics in terms of percentage("Utilization") or direct value("AverageValue")
			type: *"Utilization" | "AverageValue"
			// +usage=Specify the value of CPU utilization or averageValue
			value: *50 | int
		}
		mem?: {
			// +usage=Specify resource metrics in terms of percentage("Utilization") or direct value("AverageValue")
			type: *"Utilization" | "AverageValue"
			// +usage=Specify  the value of MEM utilization or averageValue
			value: *50 | int
		}
		// +usage=Specify custom metrics of pod type
		podCustomMetrics?: [...{
			// +usage=Specify name of custom metrics
			name: string
			// +usage=Specify target value of custom metrics
			value: string
		}]
	}
}
