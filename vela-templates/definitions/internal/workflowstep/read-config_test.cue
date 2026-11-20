import (
	"encoding/base64"
	"encoding/json"
	"vela/test"
)

"reads the named config from the step's namespace": test.#WorkflowStepExec & {
	definition: "read-config"
	context: namespace: "read-config"
	parameter: name:    "registry"
	mocks: "vela/config": "#ReadConfig": $returns: config: host: "registry.example.com"
	expect: {
		phase: "succeeded"
		calls: "vela/config": "#ReadConfig": [{$params: {name: "registry", namespace: "read-config"} @exact()}]
	}
}

"reads the config from the given namespace": test.#WorkflowStepExec & {
	definition: "read-config"
	parameter: {name: "registry", namespace: "vela-system"}
	mocks: "vela/config": "#ReadConfig": $returns: config: host: "registry.example.com"
	expect: {
		phase: "succeeded"
		calls: "vela/config": "#ReadConfig": [{$params: {name: "registry", namespace: "vela-system"}}]
	}
}

"a name is required": test.#WorkflowStepExec & {
	definition: "read-config"
	expect: {
		phase:  "failed"
		reason: "Execute"
	}
}

// A config is a Secret holding its properties as JSON under input-properties.
_config: {
	_name:      string
	_sensitive: *false | bool
	_props: {...}
	apiVersion: "v1"
	kind:       "Secret"
	metadata: {
		name: _name
		labels: "config.oam.dev/catalog": "velacore-config"
		if _sensitive {
			annotations: "config.oam.dev/sensitive": "true"
		}
	}
	data: "input-properties": base64.Encode(null, json.Marshal(_props))
}

"reads the config's properties from the cluster": test.#WorkflowStepExec & {
	definition: "read-config"
	parameter: name: "registry"
	resources: [_config & {_name: "registry", _props: {host: "registry.example.com", insecure: false}}]
	expect: {
		phase: "succeeded"
		calls: "vela/config": "#ReadConfig": [{$returns: {
			config: {host: "registry.example.com", insecure: false} @exact()
		}}]
	}
}

"a config that does not exist fails the step": test.#WorkflowStepExec & {
	definition: "read-config"
	parameter: name: "missing"
	expect: {
		phase:   "failed"
		message: =~"not found"
	}
}

"a sensitive config cannot be read": test.#WorkflowStepExec & {
	definition: "read-config"
	parameter: name: "credentials"
	resources: [_config & {_name: "credentials", _sensitive: true, _props: password: "hunter2"}]
	expect: {
		phase:   "failed"
		message: =~"sensitive"
	}
}
