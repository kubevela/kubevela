legacy: {
	type: "component"
	attributes: workload: definition: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
	}
}
template: {
	output: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		spec: template: spec: containers: [{name: "app", args: ["serve"] + parameter.extra}]
	}
	parameter: extra: *[] | [...string]
}
