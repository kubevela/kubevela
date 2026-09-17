import (
	"vela/module"
)

"module": {
	annotations: {}
	attributes: {
		workload: type: "autodetects.core.oam.dev"
		// The rendered output is the module's own Application, so this component is
		// only as healthy as that Application is. Without this, a module whose
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
				// Carry the first failing component's own message up, so the reason
				// this module is unhealthy is readable on the Application that asked
				// for it instead of only on the module's own Application.
				_first: {
					name:    *"" | string
					message: *"" | string
				}
				if len(_unhealthy) > 0 {
					_first: _unhealthy[0]
					message: "module application is \(_app.phase), component \(_first.name) unhealthy: \(_first.message)"
				}
				if len(_unhealthy) == 0 {
					if _app.phase == "" {
						message: "module application has not reported a status yet"
					}
					if _app.phase != "" {
						message: "module application is \(_app.phase)"
					}
				}
				"""#
		}
	}
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
		// Namespace the module's definitions install into; empty means the default
		// system namespace (vela-system). The Application this renders always lives
		// in vela-system whatever this is set to.
		namespace: *"" | string
		// Module package version (the OCI/ECR tag vela module publish writes from
		// _module.cue's version field). Empty means the latest published version.
		// This is not the API line (apiVersion v1/v2), which is unaffected.
		version: *"" | string
	}
}
