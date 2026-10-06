import "vela/test"

_deployment: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: name: "web"
	spec: {
		replicas: 3
		template: spec: containers: [{name: "web", image: "shop:1.0"}]
	}
}

"sets the replicas": test.#TraitRender & {
	definition: "scaler"
	workload:   _deployment
	parameter: replicas: 5
	expect: output: spec: {replicas: 5, template: spec: containers: [{name: "web"}]}
}

"scales to one by default": test.#TraitRender & {
	definition: "scaler"
	workload:   _deployment
	expect: output: spec: replicas: 1
}

"renders nothing of its own": test.#TraitRender & {
	definition: "scaler"
	workload:   _deployment
	expect: {
		outputs: {} @exact()
	}
}
