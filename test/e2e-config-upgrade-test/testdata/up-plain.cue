metadata: {
	name:        "up-plain"
	alias:       "up-plain"
	description: "config upgrade e2e"
	scope:       "namespace"
	sensitive:   false
}
template: {
	parameter: value: string
	output: {
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {name: context.name, namespace: context.namespace}
		stringData: value: parameter.value
	}
}
