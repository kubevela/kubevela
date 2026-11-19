import "vela/test"

_deployed: {
	_name: string
	workload: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: _name
		spec: selector: matchLabels: app: _name
	}
}
_web: _deployed & {_name: "web"}

"guards every component": test.#PolicyRender & {
	definition: "network-guard"
	parameter: allowFrom: "ingress"
	artifacts: {
		web: _web
		api: _deployed & {_name: "api"}
	}
	expect: {
		output: {
			metadata: {name: "test-policy-summary", namespace: "default", labels: "app.oam.dev/name": "test-app"}
			data: {web: "guarded from ingress", api: "guarded from ingress"}
		}
		outputs: {
			web: spec: podSelector: matchLabels: app: "web"
			api: metadata: {name: "api-guard", namespace: "default"}
		} @exact()
		context: {name: "test-policy", policyName: "test-policy", policyType: "network-guard", cluster: "local"}
	}
}

"names and namespace follow the context": test.#PolicyRender & {
	definition: "network-guard"
	context: {name: "guard", appName: "shop", namespace: "team-a"}
	parameter: allowFrom: "ingress"
	artifacts: web: _web
	expect: output: metadata: {name: "guard-summary", namespace: "team-a", labels: "app.oam.dev/name": "shop"}
}

"with no components there is nothing to guard": test.#PolicyRender & {
	definition: "network-guard"
	parameter: allowFrom: "ingress"
	expect: outputs: {} @exact()
}

// allowFrom is only read per component, so it is only required once there
// is one to guard. A policy's parameters are not checked as a component's
// are, so the error names where the value was needed, not the parameter.
"allowFrom is required to guard a component": test.#PolicyRender & {
	definition: "network-guard"
	artifacts: web: _web
	expect: error: =~"output.data.web: invalid interpolation: non-concrete value string"
}
