"kruise-rollout": {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Rollout workload by kruise controller."
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["*"]
		status: {
			customStatus: #"""
				message: context.outputs.rollout.status.message
				"""#
			healthPolicy: #"""
				isHealth: context.outputs.rollout.status.phase == "Healthy"
				"""#
		}
	}
}
template: {
	#CanaryStep: {
		weight?:   int
		replicas?: int | string
		duration?: int
	}
	#TrafficRouting: {
		// use context.name as service if not filled
		service:            *"" | string
		gracePeriodSeconds: *5 | int
		type:               "nginx" | "alb"
		ingress: name: *"" | string
	}
	parameter: {
		auto: *false | bool
		canary: {
			steps: [...#CanaryStep]
			trafficRoutings?: [...#TrafficRouting]
		}
	}

	outputs: rollout: {
		apiVersion: "rollouts.kruise.io/v1alpha1"
		kind:       "Rollout"
		metadata: {
			name:      context.output.metadata.name
			namespace: context.output.metadata.namespace
		}
		spec: {
			objectRef: {
				type: "workloadRef"
				workloadRef: {
					apiVersion: context.output.apiVersion
					kind:       context.output.kind
					name:       context.output.metadata.name
				}
			}
			strategy: {
				type: "canary"
				canary: {
					steps: [
						for k, v in parameter.canary.steps {
							if v.weight != _|_ {
								weight: v.weight
							}

							if v.canaryReplicas != _|_ {
								replicas: v.replicas
							}

							pause: {
								if parameter.auto {
									duration: 0
								}
								if !parameter.auto && v.duration != _|_ {
									duration: v.duration
								}
							}
						},
					]
					if parameter.canary.trafficRoutings != _|_ {
						trafficRoutings: [
							for routing in parameter.canary.trafficRoutings {
								if routing.service != "" {
									service: routing.service
								}
								if routing.service == "" {
									service: context.name
								}
								gracePeriodSeconds: routing.gracePeriodSeconds
								type:               routing.type
								ingress: {
									if routing.ingress.name != "" {
										name: routing.ingress.name
									}
									if routing.ingress.name == "" {
										name: context.name
									}
								}
							},
						]
					}
				}
			}
		}
	}
}
