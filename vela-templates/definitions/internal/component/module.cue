import (
	"vela/module"
)

"module": {
	annotations: {}
	attributes: workload: type: "autodetects.core.oam.dev"
	description: "Install a module: fetch it and render its owned Application"
	labels: {}
	type: "component"
}

template: {
	_render: module.#Render & {
		$params: {
			module:    parameter.module
			registry:  parameter.registry
			namespace: parameter.namespace
			version:   parameter.version
		}
	}

	output: _render.$returns.application

	parameter: {
		// Module name; defaults to the component name.
		module: *context.name | string
		// Registry name; empty means the configured default.
		registry: *"" | string
		// Install namespace; empty means the default system namespace (vela-system).
		namespace: *"" | string
		// Module package version (the OCI/ECR tag vela module publish writes from
		// _module.cue's version field). Empty means the latest published version.
		// This is not the API line (apiVersion v1/v2), which is unaffected.
		version: *"" | string
	}
}
