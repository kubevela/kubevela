import (
	"vela/config"
)

"delete-config": {
	type: "workflow-step"
	annotations: {
		"category": "Config Management"
	}
	labels: {}
	description: "Delete a config: the Config, and with it its Secret and outputs, or the legacy Secret and its outputs"
}
template: {
	deploy: config.#DeleteConfig & {
		$params: parameter
	}
	parameter: {
		//+usage=Specify the name of the config.
		name: string

		//+usage=Specify the namespace of the config.
		namespace: *context.namespace | string
	}
}
