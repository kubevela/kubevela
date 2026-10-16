import (
	"vela/config"
)

"list-config": {
	type: "workflow-step"
	annotations: {
		"category": "Config Management"
	}
	labels: {}
	description: "List the configs of a template, Config-backed and legacy, each once"
}
template: {
	output: config.#ListConfig & {
		$params: parameter
	}
	parameter: {
		//+usage=Specify the template of the config.
		template: string
		//+usage=Specify the namespace of the config.
		namespace: *context.namespace | string
	}
}
