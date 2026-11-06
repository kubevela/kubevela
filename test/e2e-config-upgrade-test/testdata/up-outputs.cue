metadata: {
	name:        "up-outputs"
	alias:       "up-outputs"
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
	outputs: extra: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: {name: context.name + "-extra", namespace: context.namespace}
		data: value: parameter.value
	}
}
