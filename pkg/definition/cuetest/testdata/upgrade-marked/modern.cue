import "list"

modern: {
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
		spec: template: spec: containers: [{name: "app", args: list.Concat([["serve"], parameter.extra])}]
	}
	parameter: extra: *[] | [...string]
}
