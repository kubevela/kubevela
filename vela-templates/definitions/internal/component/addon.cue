import (
	"vela/addon"
)

"addon": {
	annotations: {}
	attributes: {
		workload: type: "autodetects.core.oam.dev"
		// The rendered output is the addon's own Application, so this component is
		// only as healthy as that Application is. Without this, an addon whose
		// Application is failing reports healthy, because a component with no
		// healthPolicy is healthy by default.
		status: {
			healthPolicy: #"""
				_app: {
					phase:    *"" | string
					services: *[] | [...{...}]
					if context.output.status != _|_ {
						if context.output.status.status != _|_ {
							phase: context.output.status.status
						}
						if context.output.status.services != _|_ {
							services: context.output.status.services
						}
					}
				}
				_unhealthy: [ for s in _app.services if !s.healthy {s}]
				isHealth: _app.phase == "running" && len(_unhealthy) == 0
				"""#
			customStatus: #"""
				_app: {
					phase:    *"" | string
					services: *[] | [...{...}]
					if context.output.status != _|_ {
						if context.output.status.status != _|_ {
							phase: context.output.status.status
						}
						if context.output.status.services != _|_ {
							services: context.output.status.services
						}
					}
				}
				_unhealthy: [ for s in _app.services if !s.healthy {s}]
				// Ready:<healthy>/<total> of the owned Application's components, the same
				// shape webservice reports, naming the first failing one so there is
				// somewhere to look. Its own message stays on its own Application.
				_ready: len(_app.services) - len(_unhealthy)
				_first: name: *"" | string
				_reason: *"" | string
				if len(_unhealthy) > 0 {
					_first:  _unhealthy[0]
					_reason: " \(_first.name) unhealthy"
				}
				if len(_unhealthy) == 0 {
					if _app.phase != "running" {
						if _app.phase != "" {
							_reason: " \(_app.phase)"
						}
					}
				}
				if len(_app.services) > 0 {
					message: "Ready:\(_ready)/\(len(_app.services))\(_reason)"
				}
				// Nothing to count yet, so the phase is all there is to report.
				if len(_app.services) == 0 {
					if _app.phase == "" {
						message: "pending"
					}
					if _app.phase != "" {
						message: _app.phase
					}
				}
				"""#
		}
	}
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
