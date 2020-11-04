output: {
	apiVersion: "standard.oam.dev/v1alpha1"
	kind:       "Autoscaler"
	spec: {
		minReplicas: parameter.minReplicas
		maxReplicas: parameter.maxReplicas
		triggers: [{
			name: parameter.name
			type: parameter.type
			condition: {
				startAt:  parameter.startAt
				duration: parameter.duration
				days:     parameter.days
				replicas: parameter.replicas
				timezone: parameter.timezone
			}
		}, ...]
	}
}
parameter: {
	minReplicas: *1 | int
	maxReplicas: *4 | int
	name:        *"" | string
	type:        *"cron" | string
	startAt:     string
	duration:    string
	days:        string
	replicas:    *"2" | string
	timezone:    *"Asia/Shanghai" | string
}
