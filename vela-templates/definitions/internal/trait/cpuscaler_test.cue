import "vela/test"

_web: {
	definition: "cpuscaler"
	context: name: "web"
	workload: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: "web"
		spec: template: spec: containers: [{name: "web", image: "shop:1.0"}]
	}
}

"scales the component's Deployment between 1 and 10 at 50% CPU": test.#TraitRender & _web & {
	expect: {
		outputs: {
			cpuscaler: {
				apiVersion: "autoscaling/v1"
				kind:       "HorizontalPodAutoscaler"
				metadata: name: "web"
				spec: {
					scaleTargetRef: {apiVersion: "apps/v1", kind: "Deployment", name: "web"}
					minReplicas:                    1
					maxReplicas:                    10
					targetCPUUtilizationPercentage: 50
				} @exact()
			}
		} @exact()
		output: {kind: "Deployment", spec: template: spec: containers: [{name: "web", image: "shop:1.0"}]}
	}
}

"bounds and target utilisation are configurable": test.#TraitRender & _web & {
	parameter: {min: 2, max: 20, cpuUtil: 80}
	expect: outputs: cpuscaler: spec: {minReplicas: 2, maxReplicas: 20, targetCPUUtilizationPercentage: 80}
}

"can target another kind of workload": test.#TraitRender & _web & {
	parameter: {targetAPIVersion: "apps/v1", targetKind: "StatefulSet"}
	expect: outputs: cpuscaler: spec: scaleTargetRef: {apiVersion: "apps/v1", kind: "StatefulSet", name: "web"}
}

"a bound must be a number": test.#TraitRender & _web & {
	parameter: max: "ten"
	expect: error: {
		parameter: [=~"max"] @contains()
	}
}
