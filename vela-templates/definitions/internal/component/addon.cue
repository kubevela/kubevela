import (
	"vela/addon"
)

"addon": {
	annotations: {}
	attributes: workload: type: "autodetects.core.oam.dev"
	description: "Install an addon as a component; the addon's Application is tracked by this Application."
	labels: {}
	type: "component"
}

template: {
	_render: addon.#Render & {
		$params: {
			addon:               parameter.addon
			version:             parameter.version
			registry:            parameter.registry
			properties:          parameter.properties
			skipVersionValidate: parameter.skipVersionValidation
		}
	}

	output: _render.$returns.application

	parameter: {
		// Addon name; defaults to the component name.
		addon: *context.name | string
		// Exact version; empty means latest stable version.
		version: *"" | string
		// Registry name; empty means the configured default.
		registry: *"" | string
		// The addon's own parameters, passed through to its templates.
		// An open struct (defaults to empty) so any addon-specific parameters
		// are accepted; kept as a plain struct (not a "*{} | {...}" union) so
		// the SDK generator produces valid code.
		properties: {...}
		// Skip the addon's vela/kubernetes SystemRequirements check (mirrors the
		// imperative skipVersionValidate escape hatch). Defaults to enforcing it.
		skipVersionValidation: *false | bool
	}
}
