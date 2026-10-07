import "vela/test"

_web: {
	definition: "hpa"
	context: {
		name:    "web"
		appName: "shop"
		clusterVersion: {major: "1", minor: _minor, gitVersion: "v1.\(_minor).0", platform: "linux/amd64"}
	}
	_minor: *29 | int
	workload: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: "web"
		spec: {
			replicas: 3
			template: spec: containers: [{name: "web", image: "shop:1.0"}]
		}
	}
}

"scales the component's Deployment on CPU utilisation": test.#TraitRender & _web & {
	expect: {
		outputs: {
			hpa: {
				apiVersion: "autoscaling/v2"
				kind:       "HorizontalPodAutoscaler"
				metadata: name: "web"
				spec: {
					scaleTargetRef: {apiVersion: "apps/v1", kind: "Deployment", name: "web"}
					minReplicas: 1
					maxReplicas: 10
					metrics: [{
						type: "Resource"
						resource: {
							name: "cpu"
							target: {type: "Utilization", averageUtilization: 50} @exact()
						}
					}]
				}
			}
		} @exact()
	}
}

"leaves the workload as it was": test.#TraitRender & _web & {
	expect: output: {
		metadata: name: "web"
		spec: {
			replicas: 3
			template: spec: containers: [{name: "web", image: "shop:1.0"}]
		} @exact()
	}
}

"clusters before 1.23 get the v2beta2 API": test.#TraitRender & _web & {
	_minor: 22
	expect: outputs: hpa: apiVersion: "autoscaling/v2beta2"
}

"replica bounds and the target are set from parameters": test.#TraitRender & _web & {
	parameter: {min: 2, max: 6, targetKind: "StatefulSet", targetAPIVersion: "apps/v1"}
	expect: outputs: hpa: spec: {
		scaleTargetRef: {kind: "StatefulSet", name: "web"}
		minReplicas: 2
		maxReplicas: 6
	}
}

"an average CPU value replaces utilisation": test.#TraitRender & _web & {
	parameter: cpu: {type: "AverageValue", value: 200}
	expect: outputs: hpa: spec: metrics: [{resource: {
		target: {type: "AverageValue", averageValue: 200} @exact()
	}}]
}

"memory adds a second resource metric": test.#TraitRender & _web & {
	parameter: mem: {type: "AverageValue", value: 512}
	expect: outputs: hpa: spec: metrics: [
		{resource: name: "cpu"},
		{type: "Resource", resource: {name: "memory", target: {type: "AverageValue", averageValue: 512} @exact()}},
	]
}

"custom pod metrics follow the resource metrics": test.#TraitRender & _web & {
	parameter: podCustomMetrics: [{name: "requests_per_second", value: "100"}]
	expect: outputs: hpa: spec: metrics: [
		{resource: name: "cpu"},
		{type: "Pods", pods: {metric: name: "requests_per_second", target: {type: "AverageValue", averageValue: "100"}}},
	]
}

"a CPU target type other than Utilization or AverageValue is rejected": test.#TraitRender & _web & {
	parameter: cpu: type: "Percentage"
	expect: error: {
		parameter: [=~"cpu.type"] @contains()
	}
}
