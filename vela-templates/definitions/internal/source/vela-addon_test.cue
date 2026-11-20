import "vela/test"

_addon: {
	apiVersion: "core.oam.dev/v1beta1"
	kind:       "Application"
	metadata: {name: "addon-fluxcd", namespace: "vela-system", labels: "addons.oam.dev/name": "fluxcd"}
	spec: components: []
	_phase: *"running" | string
	status: status: _phase
}

"a running addon": test.#SourceExec & {
	definition: "vela-addon"
	parameter: {name: "fluxcd", namespace: "vela-system"}
	resources: [_addon]
	expect: {
		output: {installed: true, running: true, app: "addon-fluxcd", namespace: "vela-system"}
		calls: "vela/kube": "#List": [{$params: filter: {namespace: "vela-system", matchingLabels: "addons.oam.dev/name": "fluxcd"}}]
	}
}

"an installed addon that is not running": test.#SourceExec & {
	definition: "vela-addon"
	parameter: {name: "fluxcd", namespace: "vela-system"}
	resources: [_addon & {_phase: "workflowSuspending"}]
	expect: output: {installed: true, running: false, app: "addon-fluxcd"}
}

"an addon that is not installed": test.#SourceExec & {
	definition: "vela-addon"
	parameter: {name: "fluxcd", namespace: "vela-system"}
	expect: output: {installed: false, running: false, app: ""}
}

"reads vela-system by default": test.#SourceExec & {
	definition: "vela-addon"
	parameter: name: "fluxcd"
	resources: [_addon]
	expect: output: {installed: true, namespace: "vela-system"}
}

"caches for a minute, failing rather than serving stale": test.#SourceExec & {
	definition: "vela-addon"
	parameter: {name: "fluxcd", namespace: "vela-system"}
	expect: storage: {ttl: "1m", onStaleFailure: "fail", keyInputs: []}
}
