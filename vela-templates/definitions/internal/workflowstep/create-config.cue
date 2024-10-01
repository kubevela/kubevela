import (
	"vela/config"
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
	deploy: config.#CreateConfig & {
		$params: parameter
	}
	parameter: {
		//+usage=Specify the name of the config.
		name: string

		//+usage=Specify the namespace of the config.
		namespace: *context.namespace | string

		//+usage=Specify the template of the config.
		template?: string

		//+usage=Specify the content of the config.
		config: {...}
	}
}
