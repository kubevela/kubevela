import "vela/test"

_web: {
	definition: "json-merge-patch"
	workload: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: {name: "web", labels: {app: "shop", tier: "frontend"}}
		spec: {
			replicas: 3
			template: spec: containers: [
				{name: "web", image: "shop:1.0"},
				{name: "proxy", image: "envoy:1.30"},
			]
		}
	}
}

"merges the parameter into the workload": test.#TraitRender & _web & {
	parameter: {metadata: labels: release: "stable", spec: replicas: 5}
	expect: output: {
		metadata: labels: {app: "shop", tier: "frontend", release: "stable"}
		spec: {
			replicas: 5
			template: spec: containers: [{name: "web"}, {name: "proxy"}]
		}
	}
}

"null removes a field": test.#TraitRender & _web & {
	parameter: metadata: labels: tier: null
	expect: output: metadata: labels: {app: "shop", tier?: _|_}
}

"a list replaces the workload's list whole": test.#TraitRender & _web & {
	parameter: spec: template: spec: containers: [{name: "web", image: "shop:2.0"}]
	expect: output: spec: template: spec: containers: [{name: "web", image: "shop:2.0"}]
}

"renders nothing of its own": test.#TraitRender & _web & {
	parameter: spec: replicas: 5
	expect: {
		outputs: {} @exact()
	}
}
