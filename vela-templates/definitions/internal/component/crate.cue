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
		// +usage=Crate name. It must match `crate` in the `_crate.cue` of the package. Defaults to the component name.
		crate: *context.name | string
		// +usage=Name of a crate registry added with `vela crate registry add`. Empty selects the only configured registry, or the one named `catalog` when several exist.
		registry: *"" | string
		// +usage=Package version, which is the OCI tag that `vela crate publish` writes from the `version` field of `_crate.cue`. Empty selects the highest published semver tag when the workflow of the Application runs. This is not the API line (`v1`, `v2`).
		version: *"" | string
		// +usage=Clusters that receive the objects that the crate marks `runtime`. Objects with no mark and definitions stay on the hub. Without `placement`, all objects install on the hub.
		placement?: {
			// +usage=Names of the clusters, as `vela cluster list` shows them. `local` is the hub. Do not set it together with `clusterLabelSelector`.
			clusters?: [...string]
			// +usage=Cluster labels to match. All labels must match. Do not set it together with `clusters`.
			clusterLabelSelector?: [string]: string
			// +usage=If true, the install succeeds when `clusterLabelSelector` matches no cluster. A name in `clusters` that is not a cluster always fails.
			allowEmpty?: bool
		}
	}
}
