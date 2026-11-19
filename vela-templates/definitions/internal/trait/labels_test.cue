import "vela/test"

_web: {
	definition: "labels"
	workload: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: {name: "web", labels: app: "shop"}
		spec: template: {
			metadata: labels: app: "shop"
			spec: containers: [{name: "web", image: "shop:1.0"}]
		}
	}
}

"labels the workload and its pods": test.#TraitRender & _web & {
	parameter: {tier: "frontend", release: "stable"}
	expect: output: {
		metadata: labels: {app: "shop", tier: "frontend", release: "stable"}
		spec: template: metadata: {
			labels: {app: "shop", tier: "frontend", release: "stable"} @exact()
		}
	}
}

"a workload without a pod template is labelled alone": test.#TraitRender & {
	definition: "labels"
	workload: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: name: "settings"
		data: mode:     "dark"
	}
	parameter: tier: "frontend"
	expect: output: {
		metadata: labels: tier: "frontend"
		spec?: _|_
	}
}

"null removes a label": test.#TraitRender & _web & {
	parameter: app: null
	expect: output: {
		metadata: labels: app?: _|_
		spec: template: metadata: {
			labels: {} @exact()
		}
	}
}

"label values must be strings": test.#TraitRender & _web & {
	parameter: replicas: 3
	expect: error: {
		parameter: [=~"replicas"] @contains()
	}
}
