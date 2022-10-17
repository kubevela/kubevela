import (
	"vela/op"
)

"read-config": {
	type: "workflow-step"
	annotations: {}
	labels: {}
	description: "Read a config"
}
template: {
	output: op.#ReadConfig & {
		name: parameter.name
		if parameter.namespace != _|_ {
			namespace: parameter.namespace
		}
	}
	parameter: {
		//+usage=Specify the name of the config.
		name: string

		//+usage=Specify the namespace of the config.
		namespace?: string
	}
}
