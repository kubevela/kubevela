import (
	"vela/crate"
)

"crate": {
	// What this renders is fetched from a registry and can change while the
	// properties stay the same (a new tag, or a tag re-pushed), so every workflow
	// run applies it again rather than only when the properties change.
	annotations: "definition.oam.dev/redispatch-on-workflow-run": "true"
	attributes: {
		workload: type: "autodetects.core.oam.dev"
		// The rendered output is the crate's own Application, so this component is
		// only as healthy as that Application is. Without this, a crate whose
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
	description: "Install a crate: fetch it and render its owned Application"
	labels: {}
	type: "component"
}

template: {
	// The definitions install into the namespace of the Application that
	// installs this crate, never one a property picks. An addon's owned
	// Application always lives in vela-system, so the addon renderer records
	// the installing Application's namespace on it in an annotation; that is
	// honored only on an owned addon Application, so a user's own Application
	// cannot use it to redirect a crate.
	_namespace: string
	_fromAddon: context.appLabels["addons.oam.dev/name"] != _|_ &&
		context.appAnnotations["crates.oam.dev/install-namespace"] != _|_
	if _fromAddon {
		_namespace: context.appAnnotations["crates.oam.dev/install-namespace"]
	}
	if !_fromAddon {
		_namespace: context.namespace
	}

	// namespace is refused: a crate's definitions install into the namespace
	// of the Application that installs it, never one a property picks. A
	// component setting it must fail rather than install somewhere its
	// author did not ask for.
	if parameter.namespace != _|_ {
		_namespace: "namespace is no longer accepted: a crate's definitions install into the namespace of the Application that installs it"
	}

	// The crate provider checks placement: a set placement names clusters or
	// a clusterLabelSelector, not both. It runs during admission too, so the
	// check is not repeated here, where CUE can only report it as a type
	// conflict.
	_render: crate.#Render & {
		$params: {
			crate:     parameter.crate
			registry:  parameter.registry
			namespace: _namespace
			version:   parameter.version
			if parameter.placement != _|_ {
				placement: parameter.placement
			}
		}
	}

	output: _render.$returns.application

	parameter: {
		// Crate name; defaults to the component name.
		crate: *context.name | string
		// Registry name; empty means the configured default.
		registry: *"" | string
		// Crate package version (the OCI/ECR tag vela crate publish writes from
		// _crate.cue's version field). Empty means the latest published version.
		// This is not the API line (apiVersion v1/v2), which is unaffected.
		version: *"" | string
		// Where objects marked runtime are installed. Unmarked objects and
		// definitions stay on the hub. Set clusters or clusterLabelSelector.
		placement?: {
			clusters?: [...string]
			clusterLabelSelector?: [string]: string
			allowEmpty?: bool
		}
	}
}
