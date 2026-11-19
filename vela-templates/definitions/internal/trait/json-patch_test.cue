import "vela/test"

_web: {
	definition: "json-patch"
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

"applies each operation in order": test.#TraitRender & _web & {
	parameter: operations: [
		{op: "replace", path: "/spec/replicas", value: 5},
		{op: "add", path: "/spec/template/spec/containers/1/args", value: ["--verbose"]},
		{op: "replace", path: "/spec/template/spec/containers/1/args", value: ["--log-level=debug"]},
		{op: "remove", path: "/metadata/labels/tier"},
	]
	expect: output: {
		metadata: labels: {app: "shop", tier?: _|_}
		spec: {
			replicas: 5
			template: spec: containers: [
				{name: "web", args?: _|_},
				{name: "proxy", args: ["--log-level=debug"]},
			]
		}
	}
}

"adds to the end of a list with a dash": test.#TraitRender & _web & {
	parameter: operations: [{op: "add", path: "/spec/template/spec/containers/-", value: {name: "logs", image: "fluent-bit:3.0"}}]
	expect: output: spec: template: spec: containers: [{name: "web"}, {name: "proxy"}, {name: "logs"}]
}

"a failing test operation fails the render": test.#TraitRender & _web & {
	parameter: operations: [
		{op: "test", path: "/spec/replicas", value: 1},
		{op: "replace", path: "/spec/replicas", value: 5},
	]
	expect: error: =~"test"
}

"removing a field that does not exist fails the render": test.#TraitRender & _web & {
	parameter: operations: [{op: "remove", path: "/spec/strategy"}]
	expect: error: =~"strategy"
}

"operations must be a list": test.#TraitRender & _web & {
	parameter: operations: "replace"
	expect: error: parameter: [=~"operations"]
}
