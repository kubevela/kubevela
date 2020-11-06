output: {
	apiVersion: "standard.oam.dev/v1alpha1"
	kind:       "Autoscaler"
	spec: {
		minReplicas: parameter.min
		maxReplicas: parameter.max
		if parameter["cpu"] != _|_ && parameter["cron"] != _|_ {
			triggers: [cpuScaler, cronScaler]
		}
		if parameter["cpu"] != _|_ && parameter["cron"] == _|_ {
			triggers: [cpuScaler]
		}
		if parameter["cpu"] == _|_ && parameter["cron"] != _|_ {
			triggers: [cronScaler]
		}
	}
}

cpuScaler: {
	type: "cpu"
	condition: {
		type: "Utilization"
		if parameter["cpu"] != _|_ {
			value: parameter.cpu
		}
	}
}

cronScaler: {
	type: "cron"
	if parameter["cron"] != _|_ {
		condition: parameter.cron
	}
}

parameter: {
	// +usage=minimal replicas of the workload
	min: int
	// +usage=maximal replicas of the workload
	max: int
	// +usage=specify the value for CPU utilization, like 80, which means 80%
	cpu?: string
	// +usage=just for `appfile`, not available for Cli usage
	cron?: {
		startAt:  string
		duration: string
		// +usage=several workdays or weekends, like "Monday, Tuesday"
		days:     string
		replicas: string
		// +usage=timezone, like "America/Seattle"
		timezone: string
	}
}
