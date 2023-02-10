import (
	"vela/op"
)

"create-config": {
	type: "workflow-step"
	annotations: {
		"category": "Config Management"
	}
	labels: {}
	description: "Create or update a config"
}
template: {
	deploy: op.#CreateConfig & {
		name: parameter.name
		if parameter.namespace != _|_ {
			namespace: parameter.namespace
		}
		if parameter.namespace == _|_ {
			namespace: context.namespace
		}
		if parameter.template != _|_ {
			template: parameter.template
		}
		config: parameter.config
	}
	parameter: {
		//+usage=Specify the name of the config.
		name: string

		//+usage=Specify the namespace of the config.
		namespace?: string

		//+usage=Specify the template of the config.
		template?: string

		//+usage=Specify the content of the config.
		config: {...}
	}
}
