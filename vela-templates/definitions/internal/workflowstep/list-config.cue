import (
	"vela/op"
)

"list-config": {
	type: "workflow-step"
	annotations: {
		"category": "Config Management"
	}
	labels: {}
	description: "List the configs"
}
template: {
	output: op.#ListConfig & {
		if parameter.namespace != _|_ {
			namespace: parameter.namespace
		}
		if parameter.namespace == _|_ {
			namespace: context.namespace
		}
		template: parameter.template
	}
	parameter: {
		//+usage=Specify the template of the config.
		template: string
		//+usage=Specify the namespace of the config.
		namespace?: string
	}
}
