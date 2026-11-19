import "vela/test"

_listed: mocks: "vela/config": "#ListConfig": $returns: configs: [{name: "hub", config: registry: "hub.example.com"}]

"lists the configs of the template in the step's namespace": test.#WorkflowStepExec & _listed & {
	definition: "list-config"
	context: namespace:  "configs"
	parameter: template: "image-registry"
	expect: {
		phase: "succeeded"
		calls: "vela/config": "#ListConfig": [{$params: {template: "image-registry", namespace: "configs"} @exact()}]
	}
}

"lists from the given namespace": test.#WorkflowStepExec & _listed & {
	definition: "list-config"
	parameter: {template: "vela-system/email", namespace: "configs-elsewhere"}
	expect: {
		phase: "succeeded"
		calls: "vela/config": "#ListConfig": [{$params: {template: "vela-system/email", namespace: "configs-elsewhere"}}]
	}
}

"the template is required": test.#WorkflowStepExec & _listed & {
	definition: "list-config"
	expect: {
		phase:   "failed"
		message: =~"template"
	}
}

_config: {
	_name:      string
	apiVersion: "v1"
	kind:       "Secret"
	metadata: {
		name: _name
		labels: {
			"config.oam.dev/catalog": "velacore-config"
			"config.oam.dev/type":    "image-registry"
		}
	}
	stringData: "input-properties": "{\"registry\":\"\(_name).example.com\"}"
}

"lists the config Secrets of the template from the cluster": test.#WorkflowStepExec & {
	definition: "list-config"
	context: namespace:  "configs-real"
	parameter: template: "image-registry"
	resources: [
		_config & {_name: "hub"},
		// A Secret not labelled as a config is not listed.
		{apiVersion: "v1", kind: "Secret", metadata: {name: "plain", labels: "config.oam.dev/type": "image-registry"}},
	]
	expect: {
		phase: "succeeded"
		calls: "vela/config": "#ListConfig": [{$returns: configs: [{name: "hub", config: registry: "hub.example.com"}]}]
	}
}
