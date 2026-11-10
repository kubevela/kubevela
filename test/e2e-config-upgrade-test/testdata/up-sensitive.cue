metadata: {
	name:        "up-sensitive"
	alias:       "up-sensitive"
	description: "config upgrade e2e"
	scope:       "namespace"
	sensitive:   true
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
