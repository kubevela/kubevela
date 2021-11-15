"health": {
	annotations: {}
	attributes: {}
	description: "Apply periodical health checking to the application."
	labels: {}
	attributes: manageHealthCheck: true
	type: "policy"
}

template: {
	output: {
		apiVersion: "core.oam.dev/v1alpha2"
		kind:       "HealthScope"
		spec: {
			"probe-timeout":  parameter.probeTimeout
			"probe-interval": parameter.probeInterval
			appReferences: [{
				appName: context.appName
			}]
			workloadRefs: []
			manageHealthCheck: true
		}
	}
	parameter: {
		// +usage=Specify health checking timeout(seconds), default 10s
		probeTimeout: *10 | int
		// +usage=Specify health checking interval(seconds), default 30s
		probeInterval: *30 | int
	}
}
