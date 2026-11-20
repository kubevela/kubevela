import "vela/op"

"reads-legacy": {
	type:        "workflow-step"
	description: "Reads a ConfigMap through a legacy vela/op function"
}
template: {
	read: op.#Read & {
		value: {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: {name: parameter.name, namespace: context.namespace}
		}
	}
	parameter: name: string
}
