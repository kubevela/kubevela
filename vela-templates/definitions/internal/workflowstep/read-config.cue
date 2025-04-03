import (
	"vela/config"
)

"read-config": {
	type: "workflow-step"
	annotations: {
		"category": "Config Management"
	}
	labels: {}
	description: "Read a config"
}
template: {
	output: config.#ReadConfig & {
		$params: parameter
	}
	parameter: {
		//+usage=Specify the name of the config.
		name: string

		//+usage=Specify the namespace of the config.
		namespace: *context.namespace | string
	}
}
