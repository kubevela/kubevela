"health-scope-binding": {
	annotations: {}
	attributes: {}
	description: "Bind components to health scope. It will create a health scope with the same name as Policy name."
	labels: {}
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
				if parameter.boundComponents == _|_ {
					compReferences: [
						for v in context.components {
							if context.artifacts[v.name].ready {
								compName: v.name
								workload: {
									apiVersion: context.artifacts[compName].workload.apiVersion
									kind:       context.artifacts[compName].workload.kind
									name:       compName
								}
							}
						},
					]
				}
				if parameter.boundComponents != _|_ {
					compReferences: [
						for v in parameter.boundComponents
						if context.artifacts[v] != _|_ {
							if context.artifacts[v].ready {
								compName: v
								workload: {
									apiVersion: context.artifacts[compName].workload.apiVersion
									kind:       context.artifacts[compName].workload.kind
									name:       compName
								}
							}
						},
					]
				}
			}]
			workloadRefs: []
		}
	}
	parameter: {
		// +usage=Specify health checking timeout(seconds), default 10s
		probeTimeout: *10 | int
		// +usage=Specify health checking interval(seconds), default 30s
		probeInterval: *30 | int
		// +usage=Specify components to be bound with the scope, invalid
		// component name will be omitted, default all components
		boundComponents?: [...string]
	}
}
